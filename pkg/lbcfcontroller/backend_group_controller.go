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

	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/clientset/versioned"
	lbcflister "git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
)

func newBackendGroupController(
	client lbcfclient.Interface,
	lbLister lbcflister.LoadBalancerLister,
	bgLister lbcflister.BackendGroupLister,
	brLister lbcflister.BackendRecordLister,
	podLister corev1.PodLister,
	svcLister corev1.ServiceLister,
	nodeLister corev1.NodeLister) *backendGroupController {
	return &backendGroupController{
		client:              client,
		lbLister:            lbLister,
		bgLister:            bgLister,
		brLister:            brLister,
		podLister:           podLister,
		serviceLister:       svcLister,
		nodeLister:          nodeLister,
		relatedLoadBalancer: &sync.Map{},
		relatedPod:          &sync.Map{},
	}
}

type backendGroupController struct {
	client lbcfclient.Interface

	lbLister      lbcflister.LoadBalancerLister
	bgLister      lbcflister.BackendGroupLister
	brLister      lbcflister.BackendRecordLister
	podLister     corev1.PodLister
	serviceLister corev1.ServiceLister
	nodeLister    corev1.NodeLister

	relatedLoadBalancer *sync.Map
	relatedPod          *sync.Map
}

func (c *backendGroupController) syncBackendGroup(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	group, err := c.bgLister.BackendGroups(namespace).Get(name)
	if errors.IsNotFound(err) {
		return util.FinishedResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if group.DeletionTimestamp != nil {
		// BackendGroups will be deleted by K8S GC
		return util.FinishedResult()
	}

	// compare graph
	lb, err := c.lbLister.LoadBalancers(namespace).Get(group.Spec.LBName)
	if errors.IsNotFound(err) {
		return c.deleteAllBackend(namespace, group.Spec.LBName, group.Name)
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if lb.DeletionTimestamp != nil {
		return c.deleteAllBackend(namespace, group.Spec.LBName, group.Name)
	}
	if !util.LBCreated(lb) {
		return util.FinishedResult()
	}

	var expectedBackends []*lbcfapi.BackendRecord
	if group.Spec.Pods != nil {
		expectedBackends, err = c.expectedPodBackends(group, lb)
	} else if group.Spec.Service != nil {
		expectedBackends, err = c.expectedServiceBackends(group, lb)
	} else {
		expectedBackends, err = c.expectedStaticBackends(group, lb)
	}
	if err != nil {
		return util.ErrorResult(err)
	}
	return c.update(group, lb, expectedBackends)
}

func (c *backendGroupController) expectedPodBackends(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer) ([]*lbcfapi.BackendRecord, error) {
	var pods []*v1.Pod
	if group.Spec.Pods.ByLabel != nil {
		var err error
		pods, err = c.podLister.List(labels.SelectorFromSet(labels.Set(group.Spec.Pods.ByLabel.Selector)))
		if err != nil {
			return nil, err
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
			pod, err := c.podLister.Pods(group.Namespace).Get(podName)
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				continue
			}
			pods = append(pods, pod)
		}
	}

	var expectedRecords []*lbcfapi.BackendRecord
	for _, pod := range util.FilterPods(pods, util.PodAvailable) {
		record := util.ConstructPodBackendRecord(lb, group, pod)
		expectedRecords = append(expectedRecords, record)
	}
	return expectedRecords, nil
}

func (c *backendGroupController) expectedServiceBackends(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer) ([]*lbcfapi.BackendRecord, error) {
	nodes, err := c.nodeLister.List(labels.SelectorFromSet(labels.Set(group.Spec.Service.NodeSelector)))
	if err != nil {
		return nil, err
	}
	svc, err := c.serviceLister.Services(group.Namespace).Get(group.Spec.Service.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if svc.DeletionTimestamp != nil || svc.Spec.Type != v1.ServiceTypeNodePort {
		return nil, nil
	}
	var expectedRecords []*lbcfapi.BackendRecord
	for _, node := range nodes {
		backend := util.ConstructServiceBackendRecord(lb, group, svc, node)
		if backend == nil {
			klog.Infof("servicePort not found in svc %s/%s. looking for: %d/%s",
				svc.Namespace, svc.Name,
				group.Spec.Service.Port.PortNumber, group.Spec.Service.Port.Protocol)
			continue
		}
		expectedRecords = append(expectedRecords, backend)
	}
	return expectedRecords, nil
}

func (c *backendGroupController) expectedStaticBackends(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer) ([]*lbcfapi.BackendRecord, error) {
	var backends []*lbcfapi.BackendRecord
	for _, sa := range group.Spec.Static {
		backends = append(backends, util.ConstructStaticBackend(lb, group, sa))
	}
	return backends, nil
}

func (c *backendGroupController) update(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer, expectedBackends []*lbcfapi.BackendRecord) *util.SyncResult {
	existingRecords, err := c.listBackendRecords(group.Namespace, lb.Name, group.Name)
	if err != nil {
		return util.ErrorResult(err)
	}
	needCreate, needUpdate, needDelete := util.CompareBackendRecords(expectedBackends, existingRecords)
	var errs util.ErrorList
	if err := util.IterateBackends(needDelete, c.deleteBackendRecord); err != nil {
		errs = append(errs, err)
	}
	if err := util.IterateBackends(needUpdate, c.updateBackendRecord); err != nil {
		errs = append(errs, err)
	}
	if err := util.IterateBackends(needCreate, c.createBackendRecord); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return util.ErrorResult(errs)
	}

	// update status
	curTotal := len(expectedBackends)
	var curRegistered int32
	for _, r := range existingRecords {
		if util.BackendRegistered(r) {
			curRegistered++
		}
	}
	if group.Status.Backends != int32(curTotal) || group.Status.RegisteredBackends != curRegistered {
		group = group.DeepCopy()
		group.Status.Backends = int32(curTotal)
		group.Status.RegisteredBackends = curRegistered
		if err := c.updateStatus(group, &group.Status); err != nil {
			return util.ErrorResult(err)
		}
	}
	return util.FinishedResult()
}

func (c *backendGroupController) listRelatedBackendGroupsForPod(pod *v1.Pod) sets.String {
	filter := func(group *lbcfapi.BackendGroup) bool {
		if util.IsPodMatchBackendGroup(group, pod) {
			return true
		}
		return false
	}
	groups, err := c.listRelatedBackendGroups(pod.Namespace, filter)
	if err != nil {
		klog.Errorf("skip pod(%s/%s) add, list backendgroup failed: %v", pod.Namespace, pod.Name, err)
		return nil
	}
	return groups
}

func (c *backendGroupController) listRelatedBackendGroupsForLB(lb *lbcfapi.LoadBalancer) sets.String {
	filter := func(group *lbcfapi.BackendGroup) bool {
		return util.IsLBMatchBackendGroup(group, lb)
	}
	groups, err := c.listRelatedBackendGroups(lb.Namespace, filter)
	if err != nil {
		klog.Errorf("skip loadbalancer(%s/%s) add, list backendgroup failed: %v", lb.Namespace, lb.Name, err)
		return nil
	}
	return groups
}

func (c *backendGroupController) listRelatedBackendGroupForSvc(svc *v1.Service) sets.String {
	filter := func(group *lbcfapi.BackendGroup) bool {
		return util.IsSvcMatchBackendGroup(group, svc)
	}
	groups, err := c.listRelatedBackendGroups(svc.Namespace, filter)
	if err != nil {
		klog.Errorf("skip svc(%s/%s) add, list backendgroup failed: %v", svc.Namespace, svc.Name, err)
		return nil
	}
	return groups
}

func (c *backendGroupController) listRelatedBackendGroups(namespace string, filter func(group *lbcfapi.BackendGroup) bool) (sets.String, error) {
	set := sets.NewString()
	groupList, err := c.bgLister.BackendGroups(namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	related := util.FilterBackendGroup(groupList, filter)
	for _, r := range related {
		key, err := controller.KeyFunc(r)
		if err != nil {
			klog.Errorf("%v", err)
			continue
		}
		set.Insert(key)
	}
	return set, nil
}

func (c *backendGroupController) createBackendRecord(record *lbcfapi.BackendRecord) error {
	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Create(record)
	if err != nil {
		return fmt.Errorf("create BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *backendGroupController) updateBackendRecord(record *lbcfapi.BackendRecord) error {
	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Update(record)
	if err != nil {
		return fmt.Errorf("update BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *backendGroupController) deleteBackendRecord(record *lbcfapi.BackendRecord) error {
	if record.DeletionTimestamp != nil {
		return nil
	}
	err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Delete(record.Name, nil)
	if err != nil {
		return fmt.Errorf("delete BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *backendGroupController) deleteAllBackend(namespace, lbName, groupName string) *util.SyncResult {
	backends, err := c.listBackendRecords(namespace, lbName, groupName)
	if err != nil {
		return util.ErrorResult(err)
	}
	var errList []error
	for _, backend := range backends {
		if err := c.deleteBackendRecord(backend); err != nil {
			errList = append(errList, err)
		}
	}
	if len(errList) > 0 {
		var msg []string
		for i, e := range errList {
			msg = append(msg, fmt.Sprintf("%d: %v", i+1, e))
		}
		return util.ErrorResult(fmt.Errorf(strings.Join(msg, "\n")))
	}
	return util.FinishedResult()
}

func (c *backendGroupController) listBackendRecords(namespace string, lbName string, groupName string) ([]*lbcfapi.BackendRecord, error) {
	label := map[string]string{
		lbcfapi.LabelLBName:    lbName,
		lbcfapi.LabelGroupName: groupName,
	}
	selector := labels.SelectorFromSet(labels.Set(label))
	list, err := c.brLister.BackendRecords(namespace).List(selector)
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.BackendRecord
	for i := range list {
		ret = append(ret, list[i])
	}
	return ret, nil
}

func (c *backendGroupController) updateStatus(group *lbcfapi.BackendGroup, status *lbcfapi.BackendGroupStatus) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		group.Status = *status
		_, updateErr := c.client.LbcfV1beta1().BackendGroups(group.Namespace).UpdateStatus(group)
		if updateErr == nil {
			return nil
		}
		if updated, err := c.client.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); err == nil {
			group = updated
		} else {
			klog.Errorf("error getting updated BackendGroup %s/%s: %v", group.Namespace, group.Name, err)
		}
		return updateErr
	})
}
