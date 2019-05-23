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

	apicore "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func newBackendController(client lbcfclient.Interface, brLister v1beta1.BackendRecordLister, driverLister v1beta1.LoadBalancerDriverLister, podLister corev1.PodLister, recorder record.EventRecorder, invoker util.WebhookInvoker) *backendController {
	return &backendController{
		client:         client,
		brLister:       brLister,
		driverLister:   driverLister,
		podLister:      podLister,
		eventRecorder:  recorder,
		webhookInvoker: invoker,
	}
}

type backendController struct {
	client lbcfclient.Interface

	brLister     v1beta1.BackendRecordLister
	driverLister v1beta1.LoadBalancerDriverLister
	podLister    corev1.PodLister

	eventRecorder record.EventRecorder

	webhookInvoker util.WebhookInvoker
}

func (c *backendController) syncBackendRecord(key string) *util.SyncResult {
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

func (c *backendController) generateBackendAddr(backend *lbcfapi.BackendRecord) *util.SyncResult {
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
			_, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
			if err != nil {
				c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedGenerateAddr", "update status failed: %v", err)
				return util.ErrorResult(err)
			}
			c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "SuccGenerateAddr", "addr: %s", rsp.BackendAddr)
			return util.SuccResult()
		case webhooks.StatusFail:
			c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedGenerateAddr", "msg: %s", rsp.Msg)
			return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
		case webhooks.StatusRunning:
			c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "CalledGenerateAddr", "msg: %s", rsp.Msg)
			delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
			return util.AsyncResult(delay)
		default:
			c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "InvalidGenerateAddr", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
			return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
		}
	}
	// TODO: generateBackendAddr for service backend
	return util.SuccResult()
}

func (c *backendController) ensureBackend(backend *lbcfapi.BackendRecord) *util.SyncResult {
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
		return util.SuccResult()
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
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
	case webhooks.StatusRunning:
		backend = backend.DeepCopy()
		// running operation doesn't update condition's status field
		status := lbcfapi.ConditionFalse
		if curCondition := util.GetBackendRecordCondition(&backend.Status, lbcfapi.BackendRegistered); curCondition != nil {
			status = curCondition.Status
		}
		util.AddBackendCondition(&backend.Status, lbcfapi.BackendRecordCondition{
			Type:               lbcfapi.BackendRegistered,
			Status:             status,
			LastTransitionTime: v1.Now(),
			Reason:             lbcfapi.ReasonOperationInProgress.String(),
			Message:            rsp.Msg,
		})
		_, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).UpdateStatus(backend)
		if err != nil {
			c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedEnsureBackend", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "CalledEnsureBackend", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "InvalidEnsureBackend", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
	}
}

func (c *backendController) deregisterBackend(backend *lbcfapi.BackendRecord) *util.SyncResult {
	if backend.Status.BackendAddr == "" {
		return c.removeFinalizer(backend)
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
		return c.removeFinalizer(backend)
	case webhooks.StatusFail:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "FailedDeregister", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(backend, apicore.EventTypeNormal, "CalledDeregister", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(backend, apicore.EventTypeWarning, "InvalidDeregister", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
	}
}

func (c *backendController) removeFinalizer(backend *lbcfapi.BackendRecord) *util.SyncResult {
	backend = backend.DeepCopy()
	backend.Finalizers = util.RemoveFinalizer(backend.Finalizers, lbcfapi.FinalizerDeregisterBackend)
	_, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).Update(backend)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.SuccResult()
}
