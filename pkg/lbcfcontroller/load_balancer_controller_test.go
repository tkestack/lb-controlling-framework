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
	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/controller"
	"strings"
	"testing"
	"time"
)

func TestLoadBalancerCreateAndEnsure(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
}

func TestLoadBalancerEnsure(t *testing.T) {
	createdAt := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: createdAt,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBCreated)
	if getCondition.LastTransitionTime != createdAt {
		t.Fatalf("create timestamp changed, this field should not be modified")
	}
}

func TestLoadBalancerReEnsure(t *testing.T) {
	timestamp := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyAlways,
	}
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
		{
			Type:               lbcfapi.LBEnsured,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBEnsured)
	if getCondition.LastTransitionTime == timestamp {
		t.Fatalf("create timestamp should be updated to current time")
	}
}

func TestLoadBalancerNoReEnsure(t *testing.T) {
	timestamp := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
		{
			Type:               lbcfapi.LBEnsured,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBEnsured)
	if getCondition.LastTransitionTime != timestamp {
		t.Fatalf("create timestamp changed, this field should not be modified")
	}
}

func TestLoadBalancerDelete(t *testing.T) {
	timestamp := v1.Now()
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.DeletionTimestamp = &timestamp
	lb.ObjectMeta.Finalizers = []string{lbcfapi.FinalizerDeleteLB}
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if len(get.Finalizers) > 0 {
		t.Fatalf("finalizers should be deleted, remaining %#v", get)
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBReadyToDelete)
	if getCondition.Status != lbcfapi.ConditionTrue {
		t.Fatalf("condition %s should be set to true, get %#v", lbcfapi.LBReadyToDelete, getCondition)
	}
}

func TestLoadBalancerDeleteWithNoFinalizer(t *testing.T) {
	timestamp := v1.Now()
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.DeletionTimestamp = &timestamp
	lb.ObjectMeta.Finalizers = []string{}
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		// all webhook invokes fail, the test will pass only if webhook is not invoked
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ, get %+v", result)
	}
}

func TestLoadBalancerDeleteFailed(t *testing.T) {
	timestamp := v1.Now()
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.DeletionTimestamp = &timestamp
	lb.ObjectMeta.Finalizers = []string{lbcfapi.FinalizerDeleteLB}
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeFailInvoker{})
	key, _ := controller.KeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %s, get %#v", lbcfapi.FinalizerDeleteLB, get)
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBReadyToDelete)
	if getCondition.Status != lbcfapi.ConditionFalse {
		t.Fatalf("condition %s should be set to false, get %#v", lbcfapi.LBReadyToDelete, getCondition)
	} else if getCondition.Reason != string(lbcfapi.ReasonOperationFailed) {
		t.Fatalf("expect Reason %s, get %s", lbcfapi.ReasonOperationFailed, getCondition.Reason)
	}
}

func TestLoadBalancerSetOperationFailed(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:   lbcfapi.LBCreated,
			Status: lbcfapi.ConditionTrue,
		},
		{
			Type:   lbcfapi.LBEnsured,
			Status: lbcfapi.ConditionTrue,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	rsp := webhooks.ResponseForFailRetryHooks{
		Status:                 webhooks.StatusFail,
		Msg:                    "fake fail message",
		MinRetryDelayInSeconds: 1029,
	}
	result, get := ctrl.setOperationFailed(lb, rsp, lbcfapi.LBEnsured)
	if !result.IsFailed() {
		t.Fatalf("expect failed result, get %#v", result)
	} else if get == nil {
		t.Fatalf("expect latest Loadbalancer, get nil")
	} else if util.LBEnsured(get) {
		t.Fatalf("expect LoadBalancer not ensured, get %#v", get.Status.Conditions)
	} else if result.GetRetryDelay().Nanoseconds() != util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds).Nanoseconds() {
		t.Fatalf("expect minRetryDelay %d, get %d", util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds).Nanoseconds(), result.GetRetryDelay().Nanoseconds())
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBEnsured)
	if getCondition == nil {
		t.Fatalf("expect condition")
	} else if getCondition.Reason != string(lbcfapi.ReasonOperationFailed) {
		t.Fatalf("expect condition Reasong %v, get %q", lbcfapi.ReasonOperationFailed, getCondition.Reason)
	} else if getCondition.Message != rsp.Msg {
		t.Fatalf("expect condition message %q, get %q", rsp.Msg, getCondition.Message)
	}
}

func TestLoadBalancerSetOperationFailedWithError(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:   lbcfapi.LBCreated,
			Status: lbcfapi.ConditionTrue,
		},
		{
			Type:   lbcfapi.LBEnsured,
			Status: lbcfapi.ConditionTrue,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset()
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	rsp := webhooks.ResponseForFailRetryHooks{
		Status: webhooks.StatusFail,
		Msg:    "fake fail message",
	}
	result, _ := ctrl.setOperationFailed(lb, rsp, lbcfapi.LBEnsured)
	if !result.IsError() {
		t.Fatalf("expect error result, get %#v", result)
	}
}

func TestLoadBalancerSetOperationRunning(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:   lbcfapi.LBCreated,
			Status: lbcfapi.ConditionTrue,
		},
		{
			Type:   lbcfapi.LBEnsured,
			Status: lbcfapi.ConditionTrue,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	rsp := webhooks.ResponseForFailRetryHooks{
		Status:                 webhooks.StatusRunning,
		Msg:                    "fake running message",
		MinRetryDelayInSeconds: 1029,
	}
	result, get := ctrl.setOperationRunning(lb, rsp, lbcfapi.LBEnsured)
	if !result.IsAsync() {
		t.Fatalf("expect async result, get %#v", result)
	} else if get == nil {
		t.Fatalf("expect latest Loadbalancer, get nil")
	} else if util.LBEnsured(get) {
		t.Fatalf("expect LoadBalancer not ensured, get %#v", get.Status.Conditions)
	} else if result.GetReEnsurePeriodic().Nanoseconds() != util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds).Nanoseconds() {
		t.Fatalf("expect minReEnsurePeriod %d, get %d", util.CalculateRetryInterval(rsp.MinRetryDelayInSeconds).Nanoseconds(), result.GetReEnsurePeriodic().Nanoseconds())
	}
	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBEnsured)
	if getCondition == nil {
		t.Fatalf("expect condition")
	} else if getCondition.Reason != string(lbcfapi.ReasonOperationInProgress) {
		t.Fatalf("expect condition Reasong %v, get %q", lbcfapi.ReasonOperationInProgress, getCondition.Reason)
	} else if getCondition.Message != rsp.Msg {
		t.Fatalf("expect condition message %q, get %q", rsp.Msg, getCondition.Message)
	}
}

func TestLoadBalancerSetOperationInvalidOperation(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:   lbcfapi.LBCreated,
			Status: lbcfapi.ConditionTrue,
		},
		{
			Type:   lbcfapi.LBEnsured,
			Status: lbcfapi.ConditionTrue,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	ctrl := NewLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		}, &fakeSuccInvoker{})
	rsp := webhooks.ResponseForFailRetryHooks{
		Status:                 "invalidStatus",
		Msg:                    "fake running message",
		MinRetryDelayInSeconds: 1029,
	}
	result, get := ctrl.setOperationInvalidResponse(lb, rsp, lbcfapi.LBEnsured)
	if !result.IsError() {
		t.Fatalf("expect error result, get %#v", result)
	} else if get == nil {
		t.Fatalf("expect latest Loadbalancer, get nil")
	} else if util.LBEnsured(get) {
		t.Fatalf("expect LoadBalancer not ensured, get %#v", get.Status.Conditions)
	}

	getCondition := util.GetLBCondition(&get.Status, lbcfapi.LBEnsured)
	if getCondition == nil {
		t.Fatalf("expect condition")
	} else if getCondition.Reason != string(lbcfapi.ReasonInvalidResponse) {
		t.Fatalf("expect condition Reasong %v, get %q", lbcfapi.ReasonInvalidResponse, getCondition.Reason)
	} else if index := strings.Index(getCondition.Message, "unknown status"); index == -1 {
		t.Fatalf("expect returned status in condition message, get %q", getCondition.Message)
	}
}
