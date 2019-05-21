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
	"k8s.io/client-go/tools/cache"
)

func newLoadBalancerController(client lbcfclient.Interface, lbLister v1beta1.LoadBalancerLister, driverLister v1beta1.LoadBalancerDriverLister, invoker util.WebhookInvoker) *loadBalancerController {
	return &loadBalancerController{
		lbcfClient:     client,
		lister:         lbLister,
		driverLister:   driverLister,
		webhookInvoker: invoker,
	}
}

type loadBalancerController struct {
	lbcfClient lbcfclient.Interface

	lister       v1beta1.LoadBalancerLister
	driverLister v1beta1.LoadBalancerDriverLister

	webhookInvoker util.WebhookInvoker
}

func (c *loadBalancerController) syncLB(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	lb, err := c.lister.LoadBalancers(namespace).Get(name)
	if errors.IsNotFound(err) {
		return util.SuccResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if lb.DeletionTimestamp != nil {
		if !util.HasFinalizer(lb.Finalizers, lbcfapi.FinalizerDeleteLB) {
			return util.SuccResult()
		}
		if util.LBReadyToDelete(lb) {
			return c.removeFinalizer(lb)
		}
		return c.deleteLoadBalancer(lb)
	}

	if !util.LBCreated(lb) {
		return c.createLoadBalancer(lb)
	}

	if util.LBNeedEnsure(lb) {
		return c.ensureLoadBalancer(lb)
	}
	return util.SuccResult()
}

func (c *loadBalancerController) createLoadBalancer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace)).Get(lb.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for LoadBalancer %s failed: %v", lb.Spec.LBDriver, lb.Name, err))
	}
	req := &webhooks.CreateLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBSpec:     lb.Spec.LBSpec,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := c.webhookInvoker.CallCreateLoadBalancer(driver, req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		cpy := lb.DeepCopy()
		if len(rsp.LBInfo) > 0 {
			cpy.Status.LBInfo = rsp.LBInfo
		} else {
			cpy.Status.LBInfo = cpy.Spec.LBSpec
		}
		util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			return util.ErrorResult(err)
		}
		return util.SuccResult()
	case webhooks.StatusFail:
		return c.setOperationFailed(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBCreated)
	case webhooks.StatusRunning:
		return c.setOperationRunning(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBCreated)
	default:
		return c.setOperationInvalidResponse(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBCreated)
	}
}

func (c *loadBalancerController) ensureLoadBalancer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace)).Get(lb.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for LoadBalancer %s failed: %v", lb.Spec.LBDriver, lb.Name, err))
	}
	req := &webhooks.EnsureLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := c.webhookInvoker.CallEnsureLoadBalancer(driver, req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		cpy := lb.DeepCopy()
		util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBEnsured,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			return util.ErrorResult(err)
		}
		if lb.Spec.EnsurePolicy != nil && lb.Spec.EnsurePolicy.Policy == lbcfapi.PolicyAlways {
			return util.PeriodicResult(util.GetDuration(lb.Spec.EnsurePolicy.MinPeriod, util.DefaultEnsurePeriod))
		}
		return util.SuccResult()
	case webhooks.StatusFail:
		return c.setOperationFailed(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBEnsured)
	case webhooks.StatusRunning:
		return c.setOperationRunning(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBEnsured)
	default:
		return c.setOperationInvalidResponse(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBEnsured)
	}
}

func (c *loadBalancerController) deleteLoadBalancer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	driver, err := c.driverLister.LoadBalancerDrivers(util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace)).Get(lb.Spec.LBDriver)
	if err != nil {
		return util.ErrorResult(fmt.Errorf("retrieve driver %q for LoadBalancer %s failed: %v", lb.Spec.LBDriver, lb.Name, err))
	}
	req := &webhooks.DeleteLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBInfo:     lb.Status.LBInfo,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := c.webhookInvoker.CallDeleteLoadBalancer(driver, req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		cpy := lb.DeepCopy()
		util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBReadyToDelete,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			return util.ErrorResult(err)
		}
		return util.SuccResult()
	case webhooks.StatusFail:
		return c.setOperationFailed(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBReadyToDelete)
	case webhooks.StatusRunning:
		return c.setOperationRunning(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBReadyToDelete)
	default:
		return c.setOperationInvalidResponse(lb, rsp.ResponseForFailRetryHooks, lbcfapi.LBReadyToDelete)
	}
}

func (c *loadBalancerController) removeFinalizer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	lb = lb.DeepCopy()
	lb.Finalizers = util.RemoveFinalizer(lb.Finalizers, lbcfapi.FinalizerDeleteLB)
	lb, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Update(lb)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.SuccResult()
}

func (c *loadBalancerController) setOperationFailed(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationFailed.String(),
		Message:            rsp.Msg,
	})
	_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
}

func (c *loadBalancerController) setOperationRunning(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	// running operation only updates condition's Reason and Message field
	status := lbcfapi.ConditionFalse
	if curCondition := util.GetLBCondition(&lb.Status, cType); curCondition != nil {
		status = curCondition.Status
	}
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             status,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationInProgress.String(),
		Message:            rsp.Msg,
	})
	_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
	return util.AsyncResult(delay)
}

func (c *loadBalancerController) setOperationInvalidResponse(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonInvalidResponse.String(),
		Message:            fmt.Sprintf("unknown status %q, msg: %s", rsp.Status, rsp.Msg),
	})
	_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
	if err != nil {
		return util.ErrorResult(err)
	}
	return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
}
