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

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func NewBackendController(client lbcfclient.Interface, brLister v1beta1.BackendRecordLister, driverLister v1beta1.LoadBalancerDriverLister, podLister corev1.PodLister, invoker util.WebhookInvoker) *BackendController {
	return &BackendController{
		client:         client,
		brLister:       brLister,
		driverLister:   driverLister,
		podLister:      podLister,
		webhookInvoker: invoker,
	}
}

type BackendController struct {
	client lbcfclient.Interface

	brLister     v1beta1.BackendRecordLister
	driverLister v1beta1.LoadBalancerDriverLister
	podLister    corev1.PodLister

	webhookInvoker util.WebhookInvoker
}

func (c *BackendController) syncBackendRecord(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	backend, err := c.brLister.BackendRecords(namespace).Get(name)
	if errors.IsNotFound(err) {
		return util.SuccResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if backend.DeletionTimestamp != nil {
		if !util.HasFinalizer(backend.Finalizers, lbcfapi.FinalizerDeregisterBackend) {
			return util.SuccResult()
		}
		if util.BackendRecordReadyToDelete(backend) {
			return c.removeFinalizer(backend)
		}
		return c.deregisterBackend(backend)
	}

	if backend.Status.BackendAddr == "" {
		return c.generateBackendAddr(backend)
	}
	if util.BackendNeedEnsure(backend) {
		return c.ensureBackend(backend)
	}
	return util.SuccResult()
}

func (c *BackendController) generateBackendAddr(backend *lbcfapi.BackendRecord) *util.SyncResult {
	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace)).Get(backend.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for BackendRecord %s failed: %v", backend.Spec.LBDriver, backend.Name, err))
	}

	if backend.Spec.PodBackendInfo != nil {
		pod, err := c.podLister.Pods(backend.Namespace).Get(backend.Spec.PodBackendInfo.Name)
		if err != nil {
			return util.ErrorResult(err)
		}
		req := &webhooks.GenerateBackendAddrRequest{
			RequestForRetryHooks: webhooks.RequestForRetryHooks{
				RecordID: string(backend.UID),
				RetryID:  string(uuid.NewUUID()),
			},
			PodBackend: &webhooks.PodBackendInGenerateAddrRequest{
				Pod:  *pod,
				Port: backend.Spec.PodBackendInfo.Port,
			},
		}
		rsp, err := c.webhookInvoker.CallGenerateBackendAddr(driver, req)
		if err != nil {
			return util.ErrorResult(err)
		}
		switch rsp.Status {
		case webhooks.StatusSucc:
			cpy := backend.DeepCopy()
			cpy.Status.BackendAddr = rsp.BackendAddr
			util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
				Type:               lbcfapi.BackendAddrGenerated,
				Status:             lbcfapi.ConditionTrue,
				LastTransitionTime: v1.Now(),
				Message:            rsp.Msg,
			})
			_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
			if err != nil {
				return util.ErrorResult(err)
			}
			return util.SuccResult()
		case webhooks.StatusFail:
			return c.setOperationFailed(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendAddrGenerated)
		case webhooks.StatusRunning:
			return c.setOperationRunning(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendAddrGenerated)
		default:
			return c.setOperationInvalidResponse(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendAddrGenerated)
		}
	}
	// TODO: generateBackendAddr for service backend
	return util.SuccResult()
}

func (c *BackendController) ensureBackend(backend *lbcfapi.BackendRecord) *util.SyncResult {
	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace)).Get(backend.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for BackendRecord %s failed: %v", backend.Spec.LBDriver, backend.Name, err))
	}

	req := &webhooks.BackendOperationRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(backend.UID),
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
		cpy := backend.DeepCopy()
		if len(rsp.InjectedInfo) > 0 {
			cpy.Status.InjectedInfo = rsp.InjectedInfo
		}
		result := c.setOperationSucc(cpy, rsp.ResponseForFailRetryHooks, lbcfapi.BackendRegistered)
		if result.IsError() {
			return result
		}
		if cpy.Spec.EnsurePolicy != nil && cpy.Spec.EnsurePolicy.Policy == lbcfapi.PolicyAlways {
			return util.PeriodicResult(util.GetDuration(cpy.Spec.EnsurePolicy.MinPeriod, util.DefaultEnsurePeriod))
		}
		return util.SuccResult()
	case webhooks.StatusFail:
		return c.setOperationFailed(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendRegistered)
	case webhooks.StatusRunning:
		return c.setOperationRunning(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendRegistered)
	default:
		return c.setOperationInvalidResponse(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendRegistered)
	}
}

func (c *BackendController) deregisterBackend(backend *lbcfapi.BackendRecord) *util.SyncResult {
	if backend.Status.BackendAddr == "" {
		return util.SuccResult()
	}

	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace)).Get(backend.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for BackendRecord %s failed: %v", backend.Spec.LBDriver, backend.Name, err))
	}
	req := &webhooks.BackendOperationRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(backend.UID),
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
		cpy := backend.DeepCopy()
		cpy.Status.BackendAddr = ""
		util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
			Type:               lbcfapi.BackendRegistered,
			Status:             lbcfapi.ConditionFalse,
			LastTransitionTime: v1.Now(),
			Reason:             "Deregistered",
		})
		util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
			Type:               lbcfapi.BackendReadyToDelete,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
		})
		_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			return util.ErrorResult(err)
		}
		return util.SuccResult()
	case webhooks.StatusFail:
		return c.setOperationFailed(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendReadyToDelete)
	case webhooks.StatusRunning:
		return c.setOperationRunning(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendReadyToDelete)
	default:
		return c.setOperationInvalidResponse(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendReadyToDelete)
	}
}

func (c *BackendController) removeFinalizer(backend *lbcfapi.BackendRecord) *util.SyncResult {
	backend = backend.DeepCopy()
	backend.Finalizers = util.RemoveFinalizer(backend.Finalizers, lbcfapi.FinalizerDeregisterBackend)
	_, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).Update(backend)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.SuccResult()
}

func (c *BackendController) setOperationSucc(backend *lbcfapi.BackendRecord, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.BackendRecordConditionType) *util.SyncResult {
	cpy := backend.DeepCopy()
	util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: v1.Now(),
		Message:            rsp.Msg,
	})
	_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.SuccResult()
}

func (c *BackendController) setOperationFailed(backend *lbcfapi.BackendRecord, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.BackendRecordConditionType) *util.SyncResult {
	cpy := backend.DeepCopy()
	util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationFailed.String(),
		Message:            rsp.Msg,
	})
	_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
}

func (c *BackendController) setOperationRunning(backend *lbcfapi.BackendRecord, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.BackendRecordConditionType) *util.SyncResult {
	cpy := backend.DeepCopy()
	// running operation only updates condition's Reason and Message field
	status := lbcfapi.ConditionFalse
	if curCondition := util.GetBackendRecordCondition(&backend.Status, cType); curCondition != nil {
		status = curCondition.Status
	}
	util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
		Type:               cType,
		Status:             status,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationInProgress.String(),
		Message:            rsp.Msg,
	})
	_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
	return util.AsyncResult(delay)
}

func (c *BackendController) setOperationInvalidResponse(backend *lbcfapi.BackendRecord, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.BackendRecordConditionType) *util.SyncResult {
	cpy := backend.DeepCopy()
	util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonInvalidResponse.String(),
		Message:            fmt.Sprintf("unknown status %q, msg: %s", rsp.Status, rsp.Msg),
	})
	_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
}
