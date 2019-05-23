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
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func newLoadBalancerController(client lbcfclient.Interface, lbLister v1beta1.LoadBalancerLister, driverLister v1beta1.LoadBalancerDriverLister, recorder record.EventRecorder, invoker util.WebhookInvoker) *loadBalancerController {
	return &loadBalancerController{
		lbcfClient:     client,
		lister:         lbLister,
		driverLister:   driverLister,
		eventRecorder:  recorder,
		webhookInvoker: invoker,
	}
}

type loadBalancerController struct {
	lbcfClient lbcfclient.Interface

	lister       v1beta1.LoadBalancerLister
	driverLister v1beta1.LoadBalancerDriverLister

	eventRecorder  record.EventRecorder
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
		lb = lb.DeepCopy()
		if len(rsp.LBInfo) > 0 {
			lb.Status.LBInfo = rsp.LBInfo
		} else {
			lb.Status.LBInfo = lb.Spec.LBSpec
		}
		util.AddLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		util.AddLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(lb.Namespace).UpdateStatus(lb)
		if err != nil {
			c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "FailedCreateLoadBalancer", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(lb, apicore.EventTypeNormal, "SuccCreateLoadBalancer", "Successfully created load balancer")
		return util.SuccResult()
	case webhooks.StatusFail:
		c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "FailedCreateLoadBalancer", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(lb, apicore.EventTypeNormal, "CalledCreateLoadBalancer", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "InvalidCreateLoadBalancer", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
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
		lb = lb.DeepCopy()
		util.AddLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Message:            rsp.Msg,
		})
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(lb.Namespace).UpdateStatus(lb)
		if err != nil {
			c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "FailedEnsureLoadBalancer", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(lb, apicore.EventTypeNormal, "SuccEnsureLoadBalancer", "Successfully ensured load balancer attributes")
		if lb.Spec.EnsurePolicy != nil && lb.Spec.EnsurePolicy.Policy == lbcfapi.PolicyAlways {
			return util.PeriodicResult(util.GetDuration(lb.Spec.EnsurePolicy.MinPeriod, util.DefaultEnsurePeriod))
		}
		return util.SuccResult()
	case webhooks.StatusFail:
		lb = lb.DeepCopy()
		util.AddLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionFalse,
			LastTransitionTime: v1.Now(),
			Reason:             lbcfapi.ReasonOperationFailed.String(),
			Message:            rsp.Msg,
		})
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(lb.Namespace).UpdateStatus(lb)
		if err != nil {
			c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "FailedEnsureLoadBalancer", "update status failed: %v", err)
			return util.ErrorResult(err)
		}
		c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "FailedEnsureLoadBalancer", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(lb, apicore.EventTypeNormal, "CalledEnsureLoadBalancer", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "InvalidEnsureLoadBalancer", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
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
		return c.removeFinalizer(lb)
	case webhooks.StatusFail:
		c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "FailedDeleteLoadBalancer", "msg: %s", rsp.Msg)
		return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
	case webhooks.StatusRunning:
		c.eventRecorder.Eventf(lb, apicore.EventTypeNormal, "CalledDeleteLoadBalancer", "msg: %s", rsp.Msg)
		delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
		return util.AsyncResult(delay)
	default:
		c.eventRecorder.Eventf(lb, apicore.EventTypeWarning, "InvalidDeleteLoadBalancer", "unsupported status: %s, msg: %s", rsp.Status, rsp.Msg)
		return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
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
