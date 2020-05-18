/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */

package lbcfcontroller

import (
	"encoding/json"
	"fmt"
	"sync"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	lbcfclient "tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned"
	lbcflister "tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
)

func newBackendGroupController(
	client lbcfclient.Interface,
	driverLister lbcflister.LoadBalancerDriverLister,
	lbLister lbcflister.LoadBalancerLister,
	bgLister lbcflister.BackendGroupLister,
	brLister lbcflister.BackendRecordLister,
	podLister corev1.PodLister,
	svcLister corev1.ServiceLister,
	nodeLister corev1.NodeLister,
	invoker util.WebhookInvoker,
	recorder record.EventRecorder,
	dryRun bool) *backendGroupController {
	return &backendGroupController{
		client:              client,
		driverLister:        driverLister,
		lbLister:            lbLister,
		bgLister:            bgLister,
		brLister:            brLister,
		podLister:           podLister,
		serviceLister:       svcLister,
		nodeLister:          nodeLister,
		webhookInvoker:      invoker,
		eventRecorder:       recorder,
		relatedLoadBalancer: &sync.Map{},
		relatedPod:          &sync.Map{},
		dryRun:              dryRun,
	}
}

type backendGroupController struct {
	client lbcfclient.Interface

	driverLister  lbcflister.LoadBalancerDriverLister
	lbLister      lbcflister.LoadBalancerLister
	bgLister      lbcflister.BackendGroupLister
	brLister      lbcflister.BackendRecordLister
	podLister     corev1.PodLister
	serviceLister corev1.ServiceLister
	nodeLister    corev1.NodeLister

	webhookInvoker      util.WebhookInvoker
	eventRecorder       record.EventRecorder
	relatedLoadBalancer *sync.Map
	relatedPod          *sync.Map
	dryRun              bool
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

	var errList util.ErrorList
	var availableLBs []*lbcfapi.LoadBalancer
	for _, lbName := range group.Spec.GetLoadBalancers() {
		lb, err := c.lbLister.LoadBalancers(namespace).Get(lbName)
		lbNotFound := errors.IsNotFound(err)
		lbDeleting := err == nil && lb.DeletionTimestamp != nil
		if lbNotFound || lbDeleting {
			errList = append(errList, c.deleteAllBackend(namespace, lbName, group.Name)...)
			continue
		} else if err != nil {
			errList = append(errList,
				fmt.Errorf("get LoadBalancer %s/%s for BackendGroup %s/%s failed: %v",
					group.Namespace, lbName, group.Namespace, group.Name, err))
			event(c.eventRecorder, group, v1.EventTypeWarning, "GetLoadBalancerFailed", "%v", err)
			continue
		}
		if !util.LBCreated(lb) {
			continue
		}
		availableLBs = append(availableLBs, lb)
	}

	var expectedBackends, doNotDelete []*lbcfapi.BackendRecord
	if group.Spec.Pods != nil {
		expectedBackends, doNotDelete, err = c.expectedPodBackends(group, availableLBs)
	} else if group.Spec.Service != nil {
		expectedBackends, err = c.expectedServiceBackends(group, availableLBs)
	} else {
		expectedBackends, err = c.expectedStaticBackends(group, availableLBs)
	}
	if err := c.update(group, availableLBs, expectedBackends, doNotDelete); err != nil {
		errList = append(errList, err)
	}
	if len(errList) > 0 {
		return util.ErrorResult(errList)
	}
	return util.FinishedResult()
}

func (c *backendGroupController) expectedPodBackends(
	group *lbcfapi.BackendGroup,
	lbList []*lbcfapi.LoadBalancer) ([]*lbcfapi.BackendRecord, []*lbcfapi.BackendRecord, error) {
	var pods []*v1.Pod
	if group.Spec.Pods.ByLabel != nil {
		podsInAllNameSpaces, err := c.podLister.List(labels.SelectorFromSet(labels.Set(group.Spec.Pods.ByLabel.Selector)))
		if err != nil {
			return nil, nil, err
		}
		filter := func(p *v1.Pod) bool {
			if p.Namespace != group.Namespace {
				return false
			}
			except := sets.NewString(group.Spec.Pods.ByLabel.Except...)
			return !except.Has(p.Name)
		}
		pods = util.FilterPods(podsInAllNameSpaces, filter)
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

	// handle deregister policy
	// There are 3 available deregister policies, they specify when the pod should be deregistered:
	// 1. IfNotReady: the default one, save as K8S.
	// 2. IfNotRunning: deregister pods only if pod.status.phase is not "Running"
	// 3. Webhook: webhook "judgePodDeregister" is invoked, where drivers can implement their own policies
	var podsDoNotDereg []*v1.Pod
	notReadyPods := util.FilterPods(pods, func(pod *v1.Pod) bool {
		podNotReady := !util.PodAvailable(pod)
		podNotDeleting := pod.DeletionTimestamp == nil
		if podNotReady && podNotDeleting {
			return true
		}
		return false
	})
	if util.DeregIfNotRunning(group) {
		podsDoNotDereg = util.FilterPods(notReadyPods, util.PodAvailableByRunning)
	} else if util.DeregByWebhook(group) {
		podsDoNotDereg = judgeByDriver(group, notReadyPods, c.webhookInvoker, c.driverLister, c.eventRecorder, c.dryRun)
	}
	var expectedRecords, doNotDelete []*lbcfapi.BackendRecord
	for _, lb := range lbList {
		// the requirement of registering a Pod is always pod.status.conditions[].Ready = True
		for _, pod := range util.FilterPods(pods, util.PodAvailable) {
			expectedRecords = append(expectedRecords, util.ConstructPodBackendRecord(lb, group, pod)...)
		}
		for _, pod := range podsDoNotDereg {
			doNotDelete = append(doNotDelete, util.ConstructPodBackendRecord(lb, group, pod)...)
		}
	}
	if klog.V(3) {
		for _, r := range expectedRecords {
			klog.V(3).Infof("expect record ==> LB: %s, BG: %s/%s, Pod: %s",
				r.Spec.LBName, group.Namespace, group.Name, r.Spec.PodBackendInfo.Name)
		}
		for _, r := range doNotDelete {
			klog.V(3).Infof("not delete record ==> LB: %s, BG: %s/%s, Pod: %s",
				r.Spec.LBName, group.Namespace, group.Name, r.Spec.PodBackendInfo.Name)
		}
	}
	return expectedRecords, doNotDelete, nil
}

func (c *backendGroupController) expectedServiceBackends(
	group *lbcfapi.BackendGroup, lbList []*lbcfapi.LoadBalancer) ([]*lbcfapi.BackendRecord, error) {
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
	for _, lb := range lbList {
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
	}
	return expectedRecords, nil
}

func (c *backendGroupController) expectedStaticBackends(
	group *lbcfapi.BackendGroup, lbList []*lbcfapi.LoadBalancer) ([]*lbcfapi.BackendRecord, error) {
	var backends []*lbcfapi.BackendRecord
	for _, lb := range lbList {
		for _, sa := range group.Spec.Static {
			backends = append(backends, util.ConstructStaticBackend(lb, group, sa))
		}
	}
	return backends, nil
}

func (c *backendGroupController) update(
	group *lbcfapi.BackendGroup,
	lbList []*lbcfapi.LoadBalancer,
	expectedBackends []*lbcfapi.BackendRecord,
	doNotDelete []*lbcfapi.BackendRecord) error {
	var existingRecords []*lbcfapi.BackendRecord
	for _, lb := range lbList {
		records, err := c.listBackendRecords(group.Namespace, lb.Name, group.Name)
		if err != nil {
			return err
		}
		existingRecords = append(existingRecords, records...)
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
			return err
		}
	}

	needCreate, needUpdate, needDelete := util.CompareBackendRecords(expectedBackends, existingRecords, doNotDelete)

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
		return errs
	}
	return nil
}

func (c *backendGroupController) listRelatedBackendGroupsForPod(pod *v1.Pod) sets.String {
	filter := func(group *lbcfapi.BackendGroup) bool {
		return util.IsPodMatchBackendGroup(group, pod)
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
		key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(r)
		if err != nil {
			klog.Errorf("%v", err)
			continue
		}
		set.Insert(key)
	}
	return set, nil
}

func (c *backendGroupController) createBackendRecord(record *lbcfapi.BackendRecord) error {
	// in dry-run mode, BackendRecord is printed without being created
	if c.dryRun {
		var extraInfo string
		if klog.V(3) {
			b, _ := json.Marshal(record)
			extraInfo = fmt.Sprintf("created BackendRecord: %s", string(b))
		}
		klog.Infof("[dry-run] create BackendRecord %s/%s. %s", record.Namespace, record.Name, extraInfo)
		return nil
	}

	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Create(record)
	if err != nil {
		return fmt.Errorf("create BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *backendGroupController) updateBackendRecord(record *lbcfapi.BackendRecord) error {
	// in dry-run mode, BackendRecord is printed without being updated
	if c.dryRun {
		var extraInfo string
		if klog.V(3) {
			b, _ := json.Marshal(record)
			extraInfo = fmt.Sprintf("updated BackendRecord: %s", string(b))
		}
		klog.Infof("[dry-run] update BackendRecord %s/%s. %s", record.Namespace, record.Name, extraInfo)
		return nil
	}

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
	// in dry-run mode, BackendRecord is printed without being deleted
	if c.dryRun {
		klog.Infof("[dry-run] delete BackendRecord %s/%s", record.Namespace, record.Name)
		return nil
	}

	err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Delete(record.Name, nil)
	if err != nil {
		return fmt.Errorf("delete BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *backendGroupController) deleteAllBackend(namespace, lbName, groupName string) []error {
	backends, err := c.listBackendRecords(namespace, lbName, groupName)
	if err != nil {
		return []error{err}
	}
	var errList []error
	for _, backend := range backends {
		if err := c.deleteBackendRecord(backend); err != nil {
			errList = append(errList, err)
		}
	}
	return errList
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

// event sends event to K8S. This is a helper to be compatible with unit tests
func event(
	recorder record.EventRecorder,
	group *lbcfapi.BackendGroup,
	eventType, reason, msgFmt string,
	args ...interface{}) {
	if recorder != nil {
		recorder.Eventf(group, eventType, reason, msgFmt, args...)
	}
}

func judgeByDriver(
	group *lbcfapi.BackendGroup,
	notReadyPods []*v1.Pod,
	invoker util.WebhookInvoker,
	driverLister lbcflister.LoadBalancerDriverLister,
	recorder record.EventRecorder,
	dryRun bool) (doNotDereg []*v1.Pod) {
	if len(notReadyPods) == 0 {
		return nil
	}
	driver, err := driverLister.
		LoadBalancerDrivers(util.GetDriverNamespace(group.Spec.DeregisterWebhook.DriverName, group.Namespace)).
		Get(group.Spec.DeregisterWebhook.DriverName)
	if err != nil {
		return handleFailurePolicy(group, notReadyPods, recorder,
			fmt.Sprintf("get driver %s failed, err: %v", group.Spec.DeregisterWebhook.DriverName, err))
	}
	if dryRun && !driver.Spec.AcceptDryRunCall {
		return handleFailurePolicy(group, notReadyPods, recorder, "driver doesn't accept dry-run call")
	}
	req := &webhooks.JudgePodDeregisterRequest{
		DryRun:       dryRun,
		NotReadyPods: notReadyPods,
	}
	judgeRsp, err := invoker.CallJudgePodDeregister(driver, req)
	if err != nil {
		return handleFailurePolicy(group, notReadyPods, recorder,
			fmt.Sprintf("call webhook %s failed, err: %v", webhooks.JudgePodDeregister, err))
	}
	return judgeRsp.DoNotDeregister
}

func handleFailurePolicy(group *lbcfapi.BackendGroup, notReadyPods []*v1.Pod, recorder record.EventRecorder, msg string) (doNotDereg []*v1.Pod) {
	spec := group.Spec.DeregisterWebhook

	fp := lbcfapi.FailurePolicyDoNothing
	if group.Spec.DeregisterWebhook.FailurePolicy != nil {
		fp = *group.Spec.DeregisterWebhook.FailurePolicy
	}
	klog.Errorf("BackendGroup %s/%s use failure policy %v. reason: %s", group.Namespace, group.Name, fp, msg)
	event(recorder, group, v1.EventTypeWarning, "WebhookFailed", "use failure policy %v. reason: %s", fp, msg)

	if spec.FailurePolicy == nil || *spec.FailurePolicy == lbcfapi.FailurePolicyDoNothing {
		return notReadyPods
	} else if spec.FailurePolicy != nil && *spec.FailurePolicy == lbcfapi.FailurePolicyIfNotRunning {
		return util.FilterPods(notReadyPods, util.PodAvailableByRunning)
	}
	return nil
}
