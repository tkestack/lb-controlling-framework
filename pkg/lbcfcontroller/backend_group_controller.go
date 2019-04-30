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
	"fmt"
	"strings"
	"sync"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	lbcflister "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

func NewBackendGroupController(client *lbcfclient.Clientset, lbLister lbcflister.LoadBalancerLister, bgLister lbcflister.BackendGroupLister, brLister lbcflister.BackendRecordLister, podProvider util.PodProvider) *BackendGroupController {
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

	podProvider util.PodProvider

	relatedLoadBalancer *sync.Map
	relatedPod          *sync.Map
}

func (c *BackendGroupController) syncBackendGroup(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	group, err := c.bgLister.BackendGroups(namespace).Get(name)
	if errors.IsNotFound(err) {
		return util.SuccResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if group.DeletionTimestamp != nil {
		if !util.HasFinalizer(group.Finalizers, lbcfapi.FinalizerDeregisterBackendGroup) {
			return util.SuccResult()
		}
		if result := c.deleteAllBackend(namespace, group.Spec.LBName, group.Name); result.IsError() {
			return result
		}
		cpy := group.DeepCopy()
		cpy.Finalizers = util.RemoveFinalizer(cpy.Finalizers, lbcfapi.FinalizerDeregisterBackendGroup)
		if _, err := c.client.LbcfV1beta1().BackendGroups(namespace).Update(cpy); err != nil {
			return util.ErrorResult(err)
		}
		return util.SuccResult()
	}

	// compare graph
	lb, err := c.lbLister.LoadBalancers(namespace).Get(group.Spec.LBName)
	if errors.IsNotFound(err) {
		return c.deleteAllBackend(namespace, group.Spec.LBName, group.Name)
	} else if err != nil {
		return util.ErrorResult(err)
	} else if lb.DeletionTimestamp != nil {
		return c.deleteAllBackend(namespace, group.Spec.LBName, group.Name)
	}

	if group.Spec.Pods != nil {
		var pods []*v1.Pod
		if group.Spec.Pods.ByLabel != nil {
			pods, err = c.podProvider.Select(group.Spec.Pods.ByLabel.Selector)
			if err != nil {
				return util.ErrorResult(err)
			}
			filter := func(p *v1.Pod) bool {
				except := sets.NewString(group.Spec.Pods.ByLabel.Except...)
				if !except.Has(p.Name) {
					return true
				}
				return false
			}
			pods = util.FilterPods(pods, filter)
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
			return util.ErrorResult(err)
		}

		var expectedRecords []*lbcfapi.BackendRecord
		for _, pod := range util.FilterPods(pods, util.PodAvailable) {
			record := c.constructRecord(lb, group, pod.Name)
			expectedRecords = append(expectedRecords, record)
		}
		needCreate, needUpdate, needDelete := util.CompareBackendRecords(expectedRecords, existingRecords)
		var errs util.ErrorList
		if err := util.IterateBackends(needDelete, c.deleteBackendRecord); err != nil {
			// TODO: log
			errs = append(errs, err)
		}
		if err := util.IterateBackends(needUpdate, c.updateBackendRecord); err != nil {
			// TODO: log
			errs = append(errs, err)
		}
		if err := util.IterateBackends(needCreate, c.createBackendRecord); err != nil {
			// TODO: log
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return util.ErrorResult(errs)
		}
	}
	return util.SuccResult()
}

func (c *BackendGroupController) constructRecord(lb *lbcfapi.LoadBalancer, group *lbcfapi.BackendGroup, podName string) *lbcfapi.BackendRecord {
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakeBackendName(lb.Name, group.Name, podName, group.Spec.Pods.Port),
			Namespace: group.Namespace,
			Labels:    util.MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, "", podName, ""),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
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

func (c *BackendGroupController) deleteAllBackend(namespace, lbName, groupName string) *util.SyncResult {
	backends, err := c.listBackendRecords(namespace, lbName, groupName)
	if err != nil {
		return util.ErrorResult(err)
	}
	var errList []error
	for _, backend := range backends {
		errList = append(errList, c.deleteBackendRecord(backend))
	}
	if len(errList) > 0 {
		var msg []string
		for i, e := range errList {
			msg = append(msg, fmt.Sprintf("%d: %v", i+1, e))
		}
		return util.ErrorResult(fmt.Errorf(strings.Join(msg, "\n")))
	}
	return util.SuccResult()
}

func (c *BackendGroupController) listBackendRecords(namespace string, lbName string, groupName string) ([]*lbcfapi.BackendRecord, error) {
	label := map[string]string{
		lbcfapi.LabelLBName:    lbName,
		lbcfapi.LabelGroupName: groupName,
	}
	selector := labels.SelectorFromSet(labels.Set(label))
	list, err := c.client.LbcfV1beta1().BackendRecords(namespace).List(metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.BackendRecord
	for i := range list.Items {
		ret[i] = &list.Items[i]
	}
	return ret, nil
}

func (c *BackendGroupController) LBRelatedGroup(lb *lbcfapi.LoadBalancer) (*lbcfapi.BackendGroup, error) {
	groupList, err := c.bgLister.BackendGroups(lb.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, backendGroup := range groupList {
		if util.IsLBMatchBackendGroup(backendGroup, lb) {
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
		if util.IsPodMatchBackendGroup(backendGroup, pod) {
			return backendGroup, nil
		}
	}
	return nil, nil
}
