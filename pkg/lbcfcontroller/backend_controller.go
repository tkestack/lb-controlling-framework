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
	"fmt"
	"sync"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	apicore "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func newBackendController(client lbcfclient.Interface, brLister v1beta1.BackendRecordLister, driverLister v1beta1.LoadBalancerDriverLister, podLister corev1.PodLister, svcLister corev1.ServiceLister, nodeLister corev1.NodeLister, recorder record.EventRecorder, invoker util.WebhookInvoker) *backendController {
	return &backendController{
		client:             client,
		brLister:           brLister,
		driverLister:       driverLister,
		podLister:          podLister,
		svcLister:          svcLister,
		nodeLister:         nodeLister,
		eventRecorder:      recorder,
		inProgressDeleting: new(sync.Map),
		webhookInvoker:     invoker,
	}
}

type backendController struct {
	client        lbcfclient.Interface
	brLister      v1beta1.BackendRecordLister
	driverLister  v1beta1.LoadBalancerDriverLister
	podLister     corev1.PodLister
	svcLister     corev1.ServiceLister
	nodeLister    corev1.NodeLister
	eventRecorder record.EventRecorder

	inProgressDeleting *sync.Map
	webhookInvoker     util.WebhookInvoker
}

func (c *backendController) syncBackendRecord(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	backend, err := c.brLister.BackendRecords(namespace).Get(name)
	if errors.IsNotFound(err) {
		c.removeDeletingRecordByBackendName(namespace, name)
		return util.FinishedResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if backend.DeletionTimestamp != nil {
		if !util.HasFinalizer(backend.Finalizers, lbcfapi.FinalizerDeregisterBackend) {
			c.removeDeletingRecord(backend)
			return util.FinishedResult()
		}
		return c.deregisterBackend(backend)
	}

	if backend.Status.BackendAddr == "" {
		return c.generateBackendAddr(backend)
	}
	return c.ensureBackend(backend)
}

func (c *backendController) generateBackendAddr(backend *lbcfapi.BackendRecord) *util.SyncResult {
	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace)).Get(backend.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for BackendRecord %s failed: %v", backend.Spec.LBDriver, backend.Name, err))
	}

	var rsp *webhooks.GenerateBackendAddrResponse
	if backend.Spec.PodBackendInfo != nil {
		rsp, err = c.generatePodAddr(backend, driver)
		if err != nil {
			return util.ErrorResult(err)
		}
	} else if backend.Spec.ServiceBackendInfo != nil {
		rsp, err = c.generateServiceAddr(backend, driver)
		if err != nil {
			return util.ErrorResult(err)
		}
	} else if backend.Spec.StaticAddr != nil {
		rsp, _ = c.generateStaticAddr(backend)
	} else {
		return util.ErrorResult(fmt.Errorf("unknown backend type"))
	}

	switch rsp.Status {
	case webhooks.StatusSucc:
		cpy := backend.DeepCopy()
		cpy.Status.BackendAddr = rsp.BackendAddr
		_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedGenerateAddr", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "SuccGenerateAddr", "addr: %s", rsp.BackendAddr)
		return util.FinishedResult()
	case webhooks.StatusFail:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedGenerateAddr", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds), rsp.Msg)
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "RunningGenerateAddr", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "InvalidGenerateAddr", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
	}
}

func (c *backendController) ensureBackend(backend *lbcfapi.BackendRecord) *util.SyncResult {
	if name, deleting := c.sameAddrDeleting(backend); deleting {
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "DelayedEnsureBackend", "ensureBackend will start once %s is finished", name)
		return util.AsyncResult(util.CalculateRetryInterval(0))
	}

	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace)).Get(backend.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for BackendRecord %s failed: %v", backend.Spec.LBDriver, backend.Name, err))
	}

	req := &webhooks.BackendOperationRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("ensureBackend(%s)", backend.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBInfo:       backend.Spec.LBInfo,
		BackendAddr:  backend.Status.BackendAddr,
		Parameters:   backend.Spec.Parameters,
		InjectedInfo: backend.Status.InjectedInfo,
	}
	rsp, err := c.webhookInvoker.CallEnsureBackend(driver, req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		backend = backend.DeepCopy()
		if len(rsp.InjectedInfo) > 0 {
			backend.Status.InjectedInfo = rsp.InjectedInfo
		}
		util.AddBackendCondition(&backend.Status, lbcfapi.BackendRecordCondition{
			Type:               lbcfapi.BackendRegistered,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		_, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).UpdateStatus(backend)
		if err != nil {
			c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedEnsureBackend", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "SuccEnsureBackend", "Successfully ensured backend")
		if backend.Spec.EnsurePolicy != nil && backend.Spec.EnsurePolicy.Policy == lbcfapi.PolicyAlways {
			return util.PeriodicResult(util.GetDuration(backend.Spec.EnsurePolicy.MinPeriod, util.DefaultEnsurePeriod))
		}
		return util.FinishedResult()
	case webhooks.StatusFail:
		backend = backend.DeepCopy()
		util.AddBackendCondition(&backend.Status, lbcfapi.BackendRecordCondition{
			Type:               lbcfapi.BackendRegistered,
			Status:             lbcfapi.ConditionFalse,
			LastTransitionTime: v1.Now(),
			Reason:             lbcfapi.ReasonOperationFailed.String(),
			Message:            rsp.Msg,
		})
		_, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).UpdateStatus(backend)
		if err != nil {
			c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedEnsureBackend", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedEnsureBackend", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds), rsp.Msg)
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "RunningEnsureBackend", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "InvalidEnsureBackend", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
	}
}

func (c *backendController) deregisterBackend(backend *lbcfapi.BackendRecord) *util.SyncResult {
	c.storeDeletingBackend(backend)

	if backend.Status.BackendAddr == "" {
		return c.removeFinalizer(backend)
	}

	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace)).Get(backend.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for BackendRecord %s failed: %v", backend.Spec.LBDriver, backend.Name, err))
	}
	req := &webhooks.BackendOperationRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("deregisterBackend(%s)", backend.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBInfo:       backend.Spec.LBInfo,
		BackendAddr:  backend.Status.BackendAddr,
		Parameters:   backend.Spec.Parameters,
		InjectedInfo: backend.Status.InjectedInfo,
	}
	rsp, err := c.webhookInvoker.CallDeregisterBackend(driver, req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		return c.removeFinalizer(backend)
	case webhooks.StatusFail:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedDeregister", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds), rsp.Msg)
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "RunningDeregister", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "InvalidDeregister", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
	}
}

func (c *backendController) removeFinalizer(backend *lbcfapi.BackendRecord) *util.SyncResult {
	c.removeDeletingRecord(backend)

	backend = backend.DeepCopy()
	backend.Finalizers = util.RemoveFinalizer(backend.Finalizers, lbcfapi.FinalizerDeregisterBackend)
	_, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).Update(backend)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.FinishedResult()
}

func (c *backendController) storeDeletingBackend(backend *lbcfapi.BackendRecord) {
	key := fmt.Sprintf("%s|%s", backend.Spec.LBInfo, backend.Status.BackendAddr)
	value := util.NamespacedNameKeyFunc(backend.Namespace, backend.Name)
	c.inProgressDeleting.Store(key, value)
}

func (c *backendController) removeDeletingRecord(backend *lbcfapi.BackendRecord) {
	key := fmt.Sprintf("%s|%s", backend.Spec.LBInfo, backend.Status.BackendAddr)
	c.inProgressDeleting.Delete(key)
}

func (c *backendController) removeDeletingRecordByBackendName(namespace, name string) {
	var remainedKey *string
	targetValue := util.NamespacedNameKeyFunc(namespace, name)
	c.inProgressDeleting.Range(func(key, value interface{}) bool {
		k := key.(string)
		v := value.(string)
		if v == targetValue {
			remainedKey = &k
			return false
		}
		return true
	})
	if remainedKey != nil {
		c.inProgressDeleting.Delete(*remainedKey)
	}
}

func (c *backendController) sameAddrDeleting(backend *lbcfapi.BackendRecord) (string, bool) {
	key := fmt.Sprintf("%s|%s", backend.Spec.LBInfo, backend.Status.BackendAddr)
	value, ok := c.inProgressDeleting.Load(key)
	if ok {
		return value.(string), ok
	}
	return "", ok
}

func (c *backendController) generatePodAddr(backend *lbcfapi.BackendRecord, driver *lbcfapi.LoadBalancerDriver) (*webhooks.GenerateBackendAddrResponse, error) {
	pod, err := c.podLister.Pods(backend.Namespace).Get(backend.Spec.PodBackendInfo.Name)
	if err != nil {
		return nil, err
	}
	req := &webhooks.GenerateBackendAddrRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("generateBackendAddr(%s)", backend.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBInfo:       backend.Spec.LBInfo,
		LBAttributes: backend.Spec.LBAttributes,
		PodBackend: &webhooks.PodBackendInGenerateAddrRequest{
			Pod:  *pod,
			Port: backend.Spec.PodBackendInfo.Port,
		},
	}
	return c.webhookInvoker.CallGenerateBackendAddr(driver, req)
}

func (c *backendController) generateServiceAddr(backend *lbcfapi.BackendRecord, driver *lbcfapi.LoadBalancerDriver) (*webhooks.GenerateBackendAddrResponse, error) {
	node, err := c.nodeLister.Get(backend.Spec.ServiceBackendInfo.NodeName)
	if err != nil {
		return nil, err
	}
	svc, err := c.svcLister.Services(backend.Namespace).Get(backend.Spec.ServiceBackendInfo.Name)
	if err != nil {
		return nil, err
	}
	req := &webhooks.GenerateBackendAddrRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("generateBackendAddr(%s)", backend.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBInfo:       backend.Spec.LBInfo,
		LBAttributes: backend.Spec.LBAttributes,
		ServiceBackend: &webhooks.ServiceBackendInGenerateAddrRequest{
			Service:       *svc,
			Port:          backend.Spec.ServiceBackendInfo.Port,
			NodeName:      node.Name,
			NodeAddresses: node.Status.Addresses,
		},
	}
	return c.webhookInvoker.CallGenerateBackendAddr(driver, req)
}

func (c *backendController) generateStaticAddr(backend *lbcfapi.BackendRecord) (*webhooks.GenerateBackendAddrResponse, error) {
	rsp := &webhooks.GenerateBackendAddrResponse{}
	rsp.Status = webhooks.StatusSucc
	rsp.BackendAddr = *backend.Spec.StaticAddr
	return rsp, nil
}
