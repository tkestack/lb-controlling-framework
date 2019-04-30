/*
 * Copyright 2019 THL A29 Limited, a Tencent company.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package lbcfcontroller

import (
	"crypto/md5"
	"fmt"
	"strings"
	"sync"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	lbcflister "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

func NewBackendGroupController(client *lbcfclient.Clientset, lbLister lbcflister.LoadBalancerLister, bgLister lbcflister.BackendGroupLister, brLister lbcflister.BackendRecordLister, podProvider PodProvider) *BackendGroupController {
	return &BackendGroupController{
		client:              client,
		lbLister:            lbLister,
		bgLister:            bgLister,
		brLister:            brLister,
		podProvider:         podProvider,
		relatedLoadBalancer: &sync.Map{},
		relatedPod:          &sync.Map{},
	}
}

type BackendGroupController struct {
	client *lbcfclient.Clientset

	lbLister lbcflister.LoadBalancerLister
	bgLister lbcflister.BackendGroupLister
	brLister lbcflister.BackendRecordLister

	podProvider PodProvider

	relatedLoadBalancer *sync.Map
	relatedPod          *sync.Map
}

func (c *BackendGroupController) syncBackendGroup(key string) *SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return &SyncResult{err: err}
	}
	group, err := c.bgLister.BackendGroups(namespace).Get(name)
	if errors.IsNotFound(err) {
		return &SyncResult{}
	} else if err != nil {
		return &SyncResult{err: err}
	}

	if group.DeletionTimestamp != nil {
		if !hasFinalizer(group.Finalizers, DeregisterBackendGroupFinalizer) {
			return &SyncResult{}
		}
		if result := c.deleteAllBackend(namespace, group.Spec.LBName, group.Name); result.err != nil {
			return result
		}
		cpy := group.DeepCopy()
		cpy.Finalizers = removeFinalizer(cpy.Finalizers, DeregisterBackendGroupFinalizer)
		if _, err := c.client.LbcfV1beta1().BackendGroups(namespace).Update(cpy); err != nil {
			return &SyncResult{err: err}
		}
		return &SyncResult{}
	}

	// compare graph
	lb, err := c.lbLister.LoadBalancers(namespace).Get(group.Spec.LBName)
	if errors.IsNotFound(err) {
		return c.deleteAllBackend(namespace, group.Spec.LBName, group.Name)
	} else if err != nil {
		return &SyncResult{err: err}
	} else if lb.DeletionTimestamp != nil {
		return c.deleteAllBackend(namespace, group.Spec.LBName, group.Name)
	}

	if group.Spec.Pods != nil {
		var pods []*v1.Pod
		if group.Spec.Pods.ByLabel != nil {
			pods, err = c.podProvider.Select(group.Spec.Pods.ByLabel.Selector)
			if err != nil {
				return &SyncResult{err: err}
			}
			filter := func(p *v1.Pod) bool {
				except := sets.NewString(group.Spec.Pods.ByLabel.Except...)
				if !except.Has(p.Name) {
					return true
				}
				return false
			}
			pods = filterPods(pods, filter)
		} else if len(group.Spec.Pods.ByName) > 0 {
			for _, podName := range group.Spec.Pods.ByName {
				pod, err := c.podProvider.GetPod(namespace, podName)
				if errors.IsNotFound(err) {
					continue
				} else if err != nil {
					// TODO: log
					continue
				}
				pods = append(pods, pod)
			}
		}

		existingRecords, err := c.listBackendRecords(namespace, lb.Name, group.Name)
		if err != nil {
			return &SyncResult{err: err}
		}

		expectedRecords := make(map[string]*lbcfapi.BackendRecord)
		for _, pod := range filterPods(pods, podAvailable) {
			record := c.constructRecord(lb, group, pod.Name)
			expectedRecords[record.Name] = record
		}

		needAdd := make(map[string]*lbcfapi.BackendRecord)
		needDelete := make(map[string]*lbcfapi.BackendRecord)
		needUpdate := make(map[string]*lbcfapi.BackendRecord)
		for k, v := range expectedRecords {
			exist, ok := existingRecords[k]
			if !ok {
				needAdd[k] = v
			}
			if needUpdateRecord(exist, v) {
				needUpdate[k] = v
			}
		}
		for k, v := range existingRecords {
			if _, ok := expectedRecords[k]; !ok {
				needDelete[k] = v
			}
		}
		var errs ErrorList
		if err := iterate(needDelete, c.deleteBackendRecord); err != nil {
			// TODO: log
			errs = append(errs, err)
		}
		if err := iterate(needUpdate, c.updateBackendRecord); err != nil {
			// TODO: log
			errs = append(errs, err)
		}
		if err := iterate(needAdd, c.createBackendRecord); err != nil {
			// TODO: log
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return &SyncResult{err: errs}
		}
	}
	return &SyncResult{}
}

func (c *BackendGroupController) constructRecord(lb *lbcfapi.LoadBalancer, group *lbcfapi.BackendGroup, podName string) *lbcfapi.BackendRecord {
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeBackendName(lb.Name, group.Name, podName, group.Spec.Pods.Port),
			Namespace: group.Namespace,
			Labels:    makeLabels(lb.Spec.LBDriver, lb.Name, group.Name, "", podName, ""),
			Finalizers: []string{
				DeregisterBackendFinalizer,
			},
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:       lb.Name,
			LBDriver:     lb.Spec.LBDriver,
			LBInfo:       lb.Status.LBInfo,
			LBAttributes: lb.Spec.Attributes,
			PodBackendInfo: &lbcfapi.PodBackendRecord{
				Name: podName,
				Port: group.Spec.Pods.Port,
			},
			Parameters:   group.Spec.Parameters,
			ResyncPolicy: group.Spec.ResyncPolicy,
		},
	}
}

func (c *BackendGroupController) createBackendRecord(record *lbcfapi.BackendRecord) error {
	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Create(record)
	if err != nil {
		return fmt.Errorf("create BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *BackendGroupController) updateBackendRecord(record *lbcfapi.BackendRecord) error {
	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Update(record)
	if err != nil {
		return fmt.Errorf("update BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *BackendGroupController) deleteBackendRecord(record *lbcfapi.BackendRecord) error {
	err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Delete(record.Name, nil)
	if err != nil {
		return fmt.Errorf("delete BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *BackendGroupController) deleteAllBackend(namespace, lbName, groupName string) *SyncResult {
	backends, err := c.listBackendRecords(namespace, lbName, groupName)
	if err != nil {
		return &SyncResult{err: err}
	}
	var errList []error
	for _, backend := range backends {
		if err := c.client.LbcfV1beta1().BackendRecords(namespace).Delete(backend.Name, nil); err != nil {
			// TODO: log
			errList = append(errList, fmt.Errorf("delete BackendRecord %s/%s failed, err: %v", namespace, backend.Name, err))
			continue
		}
	}
	if len(errList) > 0 {
		var msg []string
		for i, e := range errList {
			msg = append(msg, fmt.Sprintf("%d: %v", i+1, e))
		}
		return &SyncResult{
			err: fmt.Errorf(strings.Join(msg, "\n")),
		}
	}
	return &SyncResult{}
}

func (c *BackendGroupController) listBackendRecords(namespace string, lbName string, groupName string) (map[string]*lbcfapi.BackendRecord, error) {
	label := map[string]string{
		LabelLBName:    lbName,
		LabelGroupName: groupName,
	}
	selector := labels.SelectorFromSet(labels.Set(label))
	list, err := c.client.LbcfV1beta1().BackendRecords(namespace).List(metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	ret := make(map[string]*lbcfapi.BackendRecord)
	for i := range list.Items {
		ret[list.Items[i].Name] = &list.Items[i]
	}
	return ret, nil
}

func (c *BackendGroupController) LBRelatedGroup(lb *lbcfapi.LoadBalancer) (*lbcfapi.BackendGroup, error) {
	groupList, err := c.bgLister.BackendGroups(lb.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, backendGroup := range groupList {
		if isLBMatch(backendGroup, lb) {
			return backendGroup, nil
		}
	}
	return nil, nil
}

func (c *BackendGroupController) PodRelatedGroup(pod *v1.Pod) (*lbcfapi.BackendGroup, error) {
	groupList, err := c.bgLister.BackendGroups(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, backendGroup := range groupList {
		if isPodMatch(backendGroup, pod) {
			return backendGroup, nil
		}
	}
	return nil, nil
}

func makeBackendName(lbName, groupName, podName string, port lbcfapi.PortSelector) string {
	protocol := "TCP"
	if port.Protocol != nil {
		protocol = *port.Protocol
	}
	raw := fmt.Sprintf("%s-%s-%s-%d-%s", lbName, groupName, podName, port.PortNumber, protocol)
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", h)
}

const (
	LabelDriverName  = "lbcf.tke.cloud.tencent.com/lb-driver"
	LabelLBName      = "lbcf.tke.cloud.tencent.com/lb-name"
	LabelGroupName   = "lbcf.tke.cloud.tencent.com/backend-group"
	LabelServiceName = "lbcf.tke.cloud.tencent.com/backend-service"
	LabelPodName     = "lbcf.tke.cloud.tencent.com/backend-pod"
	LabelStaticAddr  = "lbcf.tke.cloud.tencent.com/backend-static-addr"
)

func makeLabels(driverName, lbName, groupName, svcName, podName, staticAddr string) map[string]string {
	ret := make(map[string]string)
	ret[LabelDriverName] = driverName
	ret[LabelLBName] = lbName
	ret[LabelGroupName] = groupName
	if podName != "" {
		ret[LabelPodName] = podName
	}
	return ret
}

func needUpdateRecord(curObj *lbcfapi.BackendRecord, expectObj *lbcfapi.BackendRecord) bool {
	if !equalMap(curObj.Spec.LBAttributes, expectObj.Spec.LBAttributes) {
		return true
	}
	if !equalMap(curObj.Spec.Parameters, expectObj.Spec.Parameters) {
		return true
	}
	if !equalResyncPolicy(curObj.Spec.ResyncPolicy, curObj.Spec.ResyncPolicy) {
		return true
	}
	return false
}

func iterate(all map[string]*lbcfapi.BackendRecord, handler func(*lbcfapi.BackendRecord) error) error {
	var errList []error
	for _, record := range all {
		errList = append(errList, handler(record))
	}
	return ErrorList(errList)
}

func filterPods(all []*v1.Pod, filter func(pod *v1.Pod) bool) []*v1.Pod {
	var ret []*v1.Pod
	for _, pod := range all {
		if filter(pod) {
			ret = append(ret, pod)
		}
	}
	return ret
}

func isPodMatch(group *lbcfapi.BackendGroup, pod *v1.Pod) bool {
	if group.Namespace != pod.Namespace {
		return false
	}
	if group.Spec.Pods == nil {
		return false
	}

	if group.Spec.Pods.ByLabel != nil {
		except := sets.NewString(group.Spec.Pods.ByLabel.Except...)
		if except.Has(pod.Name) {
			return false
		}
		selector := labels.SelectorFromSet(labels.Set(group.Spec.Pods.ByLabel.Selector))
		return selector.Matches(labels.Set(pod.Labels))
	}
	included := sets.NewString(group.Spec.Pods.ByName...)
	return included.Has(pod.Name)
}

func isLBMatch(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer) bool {
	if group.Namespace == lb.Namespace && group.Spec.LBName == lb.Name {
		return true
	}
	return false
}
