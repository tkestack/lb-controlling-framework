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
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
)

func newBackendGroupController(client lbcfclient.Interface, lbLister lbcflister.LoadBalancerLister, bgLister lbcflister.BackendGroupLister, brLister lbcflister.BackendRecordLister, podLister corev1.PodLister) *backendGroupController {
	return &backendGroupController{
		client:              client,
		lbLister:            lbLister,
		bgLister:            bgLister,
		brLister:            brLister,
		podLister:           podLister,
		relatedLoadBalancer: &sync.Map{},
		relatedPod:          &sync.Map{},
	}
}

type backendGroupController struct {
	client lbcfclient.Interface

	lbLister  lbcflister.LoadBalancerLister
	bgLister  lbcflister.BackendGroupLister
	brLister  lbcflister.BackendRecordLister
	podLister corev1.PodLister

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
		return util.SuccResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if group.DeletionTimestamp != nil {
		// BackendGroups will be deleted by K8S GC
		return util.SuccResult()
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
	if !util.LBEnsured(lb) {
		return util.SuccResult()
	}

	if group.Spec.Pods != nil {
		var pods []*v1.Pod
		if group.Spec.Pods.ByLabel != nil {
			pods, err = c.podLister.List(labels.SelectorFromSet(labels.Set(group.Spec.Pods.ByLabel.Selector)))
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
				pod, err := c.podLister.Pods(namespace).Get(podName)
				if errors.IsNotFound(err) {
					continue
				} else if err != nil {
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
			record := util.ConstructBackendRecord(lb, group, pod)
			expectedRecords = append(expectedRecords, record)
		}
		needCreate, needUpdate, needDelete := util.CompareBackendRecords(expectedRecords, existingRecords)
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
		var expectRegistered int32
		for _, r := range existingRecords {
			if util.BackendRegistered(r) {
				expectRegistered++
			}
		}
		if group.Status.Backends != int32(len(expectedRecords)) || expectRegistered != group.Status.RegisteredBackends {
			group = group.DeepCopy()
			group.Status.Backends = int32(len(expectedRecords))
			group.Status.RegisteredBackends = expectRegistered
			if err := c.updateStatus(group, &group.Status); err != nil {
				return util.ErrorResult(err)
			}
		}
		return util.SuccResult()
	}
	return util.SuccResult()
}

func (c *backendGroupController) getBackendGroupsForPod(pod *v1.Pod) sets.String {
	set := sets.NewString()
	groupList, err := c.bgLister.BackendGroups(pod.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip pod(%s/%s) add, list backendgroup failed: %v", pod.Namespace, pod.Name, err)
		return nil
	}
	related := util.FilterBackendGroup(groupList, func(group *lbcfapi.BackendGroup) bool {
		if util.IsPodMatchBackendGroup(group, pod) {
			return true
		}
		return false
	})

	for _, r := range related {
		key, err := controller.KeyFunc(r)
		if err != nil {
			klog.Errorf("%v", err)
			continue
		}
		set.Insert(key)
	}
	return set
}

func (c *backendGroupController) getBackendGroupsForLoadBalancer(lb *lbcfapi.LoadBalancer) sets.String {
	set := sets.NewString()
	groupList, err := c.bgLister.BackendGroups(lb.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip loadbalancer(%s/%s) add, list backendgroup failed: %v", lb.Namespace, lb.Name, err)
	}
	related := util.FilterBackendGroup(groupList, func(group *lbcfapi.BackendGroup) bool {
		return util.IsLBMatchBackendGroup(group, lb)
	})
	for _, r := range related {
		key, err := controller.KeyFunc(r)
		if err != nil {
			klog.Errorf("%v", err)
			continue
		}
		set.Insert(key)
	}
	return set
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
	return util.SuccResult()
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
