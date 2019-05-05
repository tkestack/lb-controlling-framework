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

type LoadBalancerProvider interface {
	getLoadBalancer(namespace, name string) (*lbcfapi.LoadBalancer, error)
	listLoadBalancerByDriver(driverName string, driverNamespace string) ([]*lbcfapi.LoadBalancer, error)
}

func NewLoadBalancerController(client *lbcfclient.Clientset, lister v1beta1.LoadBalancerLister, driverProvider DriverProvider) *LoadBalancerController {
	return &LoadBalancerController{
		lbcfClient:     client,
		lister:         lister,
		driverProvider: driverProvider,
	}
}

type LoadBalancerController struct {
	lbcfClient     *lbcfclient.Clientset
	lister         v1beta1.LoadBalancerLister
	driverProvider DriverProvider
}

func (c *LoadBalancerController) getLoadBalancer(namespace, name string) (*lbcfapi.LoadBalancer, error) {
	return c.lister.LoadBalancers(namespace).Get(name)
}

func (c *LoadBalancerController) listLoadBalancerByDriver(driverName string, driverNamespace string) ([]*lbcfapi.LoadBalancer, error) {
	lbList, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.LoadBalancer
	for _, lb := range lbList {
		if driverNamespace != "kube-system" && lb.Namespace != driverNamespace {
			continue
		}
		if lb.Spec.LBDriver == driverName {
			ret = append(ret, lb)
		}
	}
	return ret, nil
}

func (c *LoadBalancerController) syncLB(key string) *util.SyncResult {
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
		result := c.createLoadBalancer(lb)
		if result.IsError() || result.IsAsync() {
			return result
		}
	}
	return c.ensureLoadBalancer(lb)
}

func (c *LoadBalancerController) createLoadBalancer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	driver, exist := c.driverProvider.getDriver(util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace), lb.Spec.LBDriver)
	if !exist {
		return util.ErrorResult(fmt.Errorf("driver %q not found for LoadBalancer %s", lb.Spec.LBDriver, lb.Name))
	}
	req := &webhooks.CreateLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBSpec:     lb.Spec.LBSpec,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := driver.CallCreateLoadBalancer(req)
	if err != nil {
		return util.ErrorResult(err)
	}
	switch rsp.Status {
	case webhooks.StatusSucc:
		cpy := lb.DeepCopy()
		if len(rsp.LBInfo) >= 0 {
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
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy); err != nil {
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

func (c *LoadBalancerController) ensureLoadBalancer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	driver, exist := c.driverProvider.getDriver(util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace), lb.Spec.LBDriver)
	if !exist {
		return util.ErrorResult(fmt.Errorf("driver %q not found for LoadBalancer %s", lb.Spec.LBDriver, lb.Name))
	}
	req := &webhooks.EnsureLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := driver.CallEnsureLoadBalancer(req)
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
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy); err != nil {
			return util.ErrorResult(err)
		}
		if lb.Spec.ResyncPolicy != nil && lb.Spec.ResyncPolicy.Policy == lbcfapi.PolicyAlways {
			return util.PeriodicResult(util.GetDuration(lb.Spec.ResyncPolicy.MinPeriod, util.DefaultEnsurePeriod))
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

func (c *LoadBalancerController) deleteLoadBalancer(lb *lbcfapi.LoadBalancer) *util.SyncResult {
	driver, exist := c.driverProvider.getDriver(util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace), lb.Spec.LBDriver)
	if !exist {
		return util.ErrorResult(fmt.Errorf("driver %q not found for LoadBalancer %s", lb.Spec.LBDriver, lb.Name))
	}
	req := &webhooks.DeleteLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBInfo:     lb.Status.LBInfo,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := driver.CallDeleteLoadBalancer(req)
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
		latest, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy)
		if err != nil {
			return util.ErrorResult(err)
		}
		latest.Finalizers = util.RemoveFinalizer(latest.Finalizers, lbcfapi.FinalizerDeleteLB)
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Update(latest); err != nil {
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

func (c *LoadBalancerController) setOperationSucc(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: v1.Now(),
		Message:            rsp.Msg,
	})
	if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	if cType == lbcfapi.LBEnsured && lb.Spec.ResyncPolicy.Policy == lbcfapi.PolicyAlways {
		return util.PeriodicResult(util.GetDuration(lb.Spec.ResyncPolicy.MinPeriod, util.DefaultEnsurePeriod))
	}
	return util.SuccResult()
}

func (c *LoadBalancerController) setOperationFailed(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationFailed.String(),
		Message:            rsp.Msg,
	})
	if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	return util.FailResult(util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds))
}

func (c *LoadBalancerController) setOperationRunning(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonOperationInProgress.String(),
		Message:            rsp.Msg,
	})
	if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	delay := util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds)
	return util.AsyncResult(delay)
}

func (c *LoadBalancerController) setOperationInvalidResponse(lb *lbcfapi.LoadBalancer, rsp webhooks.ResponseForFailRetryHooks, cType lbcfapi.LoadBalancerConditionType) *util.SyncResult {
	cpy := lb.DeepCopy()
	util.AddLBCondition(&cpy.Status, lbcfapi.LoadBalancerCondition{
		Type:               cType,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: v1.Now(),
		Reason:             lbcfapi.ReasonInvalidResponse.String(),
		Message:            fmt.Sprintf("unknown status %q, msg: %s", rsp.Status, rsp.Msg),
	})
	if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(cpy.Namespace).UpdateStatus(cpy); err != nil {
		return util.ErrorResult(err)
	}
	return util.ErrorResult(fmt.Errorf("unknown status %q", rsp.Status))
}
