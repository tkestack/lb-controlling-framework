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
	"testing"
	"time"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestLoadBalancerCreate(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFinished() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "SuccCreateLoadBalancer" {
		t.Fatalf("expect reason SuccCreateLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerCreateFail(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect failed, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created=false, get status: %#v", get.Status)
	}

	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "FailedCreateLoadBalancer" {
		t.Fatalf("expect reason FailedCreateLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerCreateRunning(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsRunning() {
		t.Fatalf("expect running, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created=false, get status: %#v", get.Status)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "RunningCreateLoadBalancer" {
		t.Fatalf("expect reason RunningCreateLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerCreateInvalid(t *testing.T) {
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect error, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created=false, get status: %#v", get.Status)
	}

	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "InvalidCreateLoadBalancer" {
		t.Fatalf("expect reason InvalidCreateLoadBalancer, get %s", reason)
	}
}
func TestLoadBalancerEnsure(t *testing.T) {
	createdAt := v1.Time{Time: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
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
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFinished() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "SuccEnsureLoadBalancer" {
		t.Fatalf("expect reason SuccEnsureLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerEnsureFail(t *testing.T) {
	timestamp := v1.Time{Time: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
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
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect fail, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured=false, get status: %#v", get.Status)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "FailedEnsureLoadBalancer" {
		t.Fatalf("expect reason FailedEnsureLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerEnsureRunning(t *testing.T) {
	timestamp := v1.Time{Time: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
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
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsRunning() {
		t.Fatalf("expect fail, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured=true, get status: %#v", get.Status)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "RunningEnsureLoadBalancer" {
		t.Fatalf("expect reason RunningEnsureLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerReEnsure(t *testing.T) {
	timestamp := v1.Time{Time: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.Spec.LBDriver = "test-driver"
	lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
		{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
		{
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFinished() {
		t.Fatalf("expect finished, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if !util.LBCreated(get) {
		t.Errorf("expect LoadBalancer created, get status: %#v", get.Status)
	} else if !util.LBEnsured(get) {
		t.Errorf("expect LoadBalancer ensured, get status: %#v", get.Status)
	}
	if len(store) != 0 {
		t.Fatalf("expect 1 event, get %d", len(store))
	}
}

func TestLoadBalancerEnsureInvalid(t *testing.T) {
	timestamp := v1.Time{Time: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
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
			Type:               lbcfapi.LBAttributesSynced,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: timestamp,
		},
	}
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect error, get %+v", result)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "InvalidEnsureLoadBalancer" {
		t.Fatalf("expect reason InvalidEnsureLoadBalancer, get %s", reason)
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
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFinished() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if len(get.Finalizers) != 0 {
		t.Fatalf("expect empty finalizers, get %#v", get)
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
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect succ, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %s, get %#v", lbcfapi.FinalizerDeleteLB, get)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "FailedDeleteLoadBalancer" {
		t.Fatalf("expect reason FailedDeleteLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerDeleteRunning(t *testing.T) {
	timestamp := v1.Now()
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.DeletionTimestamp = &timestamp
	lb.ObjectMeta.Finalizers = []string{lbcfapi.FinalizerDeleteLB}
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsRunning() {
		t.Fatalf("expect async, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %s, get %#v", lbcfapi.FinalizerDeleteLB, get)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "RunningDeleteLoadBalancer" {
		t.Fatalf("expect reason RunningDeleteLoadBalancer, get %s", reason)
	}
}

func TestLoadBalancerDeleteInvalid(t *testing.T) {
	timestamp := v1.Now()
	lb := newFakeLoadBalancer("", "test-lb", nil, nil)
	lb.DeletionTimestamp = &timestamp
	lb.ObjectMeta.Finalizers = []string{lbcfapi.FinalizerDeleteLB}
	lb.Spec.LBDriver = "test-driver"
	driver := newFakeDriver(lb.Namespace, lb.Spec.LBDriver)
	fakeClient := fake.NewSimpleClientset(lb)
	store := make(map[string]string)
	ctrl := newLoadBalancerController(
		fakeClient,
		&fakeLBLister{
			get: lb,
		},
		&fakeDriverLister{
			get: driver,
		},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{},
		false)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(lb)
	result := ctrl.syncLB(key)
	if !result.IsFailed() {
		t.Fatalf("expect error, get %+v", result)
	}
	get, _ := fakeClient.LbcfV1beta1().LoadBalancers(lb.Namespace).Get(lb.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %s, get %#v", lbcfapi.FinalizerDeleteLB, get)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[lb.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", lb.Name, store)
	} else if reason != "InvalidDeleteLoadBalancer" {
		t.Fatalf("expect reason InvalidDeleteLoadBalancer, get %s", reason)
	}
}
