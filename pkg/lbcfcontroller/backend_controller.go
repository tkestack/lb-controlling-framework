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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/cache"
)

type BackendProvider interface {
	listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error)
}

func NewBackendController(client *lbcfclient.Clientset, brLister v1beta1.BackendRecordLister, driverProvider DriverProvider, podProvider util.PodProvider) *BackendController {
	return &BackendController{
		client:         client,
		brLister:       brLister,
		driverProvider: driverProvider,
		podProvider:    podProvider,
	}
}

type BackendController struct {
	client *lbcfclient.Clientset

	brLister v1beta1.BackendRecordLister

	driverProvider DriverProvider
	podProvider    util.PodProvider
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
		return c.deregisterBackend(backend)
	}

	if backend.Status.BackendAddr == "" {
		result := c.generateBackendAddr(backend)
		if result.IsError() || result.IsAsync() {
			return result
		}
	}
	return c.ensureBackend(backend)
}

func (c *BackendController) listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error) {
	recordList, err := c.brLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.BackendRecord
	for _, r := range recordList {
		if driverNamespace != "kube-system" && r.Namespace != driverNamespace {
			continue
		}
		if r.Spec.LBDriver == driverName {
			ret = append(ret, r)
		}
	}
	return ret, nil
}

func (c *BackendController) generateBackendAddr(backend *lbcfapi.BackendRecord) *util.SyncResult {
	driver, exist := c.driverProvider.getDriver(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace), backend.Spec.LBDriver)
	if !exist {
		return util.ErrorResult(fmt.Errorf("driver %q not found for BackendRecord %s", backend.Spec.LBDriver, backend.Name))
	}

	if backend.Spec.PodBackendInfo != nil {
		pod, err := c.podProvider.GetPod(backend.Namespace, backend.Spec.PodBackendInfo.Name)
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
		rsp, err := driver.CallGenerateBackendAddr(req)
		if err != nil {
			return util.ErrorResult(err)
		}
		switch rsp.Status {
		case webhooks.StatusSucc:
			cpy := backend.DeepCopy()
			cpy.Status.BackendAddr = rsp.BackendAddr
			return c.setOperationSucc(cpy, rsp.ResponseForFailRetryHooks, lbcfapi.BackendAddrGenerated)
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
	driver, exist := c.driverProvider.getDriver(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace), backend.Spec.LBDriver)
	if !exist {
		return util.ErrorResult(fmt.Errorf("driver %q not found for BackendRecord %s", backend.Spec.LBDriver, backend.Name))
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
	rsp, err := driver.CallEnsureBackend(req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		result := c.setOperationSucc(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendRegistered)
		if result.IsError() {
			return result
		}
		if backend.Spec.ResyncPolicy != nil && backend.Spec.ResyncPolicy.Policy == lbcfapi.PolicyAlways {
			return util.PeriodicResult(util.GetResyncPeriod(backend.Spec.ResyncPolicy.MinPeriod))
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

	driver, exist := c.driverProvider.getDriver(util.GetDriverNamespace(backend.Spec.LBDriver, backend.Namespace), backend.Spec.LBDriver)
	if !exist {
		return util.ErrorResult(fmt.Errorf("driver %q not found for BackendRecord %s", backend.Spec.LBDriver, backend.Name))
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
	rsp, err := driver.CallDeregisterBackend(req)
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
		latest, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			return util.ErrorResult(err)
		}
		latest.Finalizers = util.RemoveFinalizer(latest.Finalizers, lbcfapi.FinalizerDeregisterBackend)
		if _, err := c.client.LbcfV1beta1().BackendRecords(backend.Namespace).Update(latest); err != nil {
			return util.ErrorResult(err)
		}
		return &util.SyncResult{}
	case webhooks.StatusFail:
		return c.setOperationFailed(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendReadyToDelete)
	case webhooks.StatusRunning:
		return c.setOperationRunning(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendReadyToDelete)
	default:
		return c.setOperationInvalidResponse(backend, rsp.ResponseForFailRetryHooks, lbcfapi.BackendReadyToDelete)
	}
}

func (c *BackendController) setOperationSucc(backend *lbcfapi.BackendRecord, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.BackendRecordConditionType) *util.SyncResult {
	cpy := backend.DeepCopy()
	util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: v1.Now(),
		Message:            rsp.Msg,
	})
	if _, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	return &util.SyncResult{}
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
	if _, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
}

func (c *BackendController) setOperationRunning(backend *lbcfapi.BackendRecord, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.BackendRecordConditionType) *util.SyncResult {
	cpy := backend.DeepCopy()
	util.AddBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationInProgress.String(),
		Message:            rsp.Msg,
	})
	if _, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy); err != nil {
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
	if _, err := c.client.LbcfV1beta1().BackendRecords(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
}
