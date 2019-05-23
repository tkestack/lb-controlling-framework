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
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/kubernetes/pkg/controller"
)

func TestBackendGenerateAddr(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr == "" {
		t.Fatalf("expect addr not empty")
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "SuccGenerateAddr" {
		t.Fatalf("expect reason SuccGenerateAddr, get %s", reason)
	}
}

func TestBackendGenerateAddrFailed(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect failed result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr != "" {
		t.Fatalf("expect empty addr, get %v", get.Status.BackendAddr)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "FailedGenerateAddr" {
		t.Fatalf("expect reason FailedGenerateAddr, get %s", reason)
	}
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition != nil {
		t.Fatalf("expect nil condition, get %#v", ensureCondition)
	}
}

func TestBackendGenerateAddrRunning(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr != "" {
		t.Fatalf("expect empty addr, get %v", get.Status.BackendAddr)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "CalledGenerateAddr" {
		t.Fatalf("expect reason CalledGenerateAddr, get %s", reason)
	}
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition != nil {
		t.Fatalf("expect nil condition, get %#v", ensureCondition)
	}
}

func TestBackendGenerateAddrInvalidResponse(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsError() {
		t.Fatalf("expect error result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr != "" {
		t.Fatalf("expect empty addr, get %v", get.Status.BackendAddr)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "InvalidGenerateAddr" {
		t.Fatalf("expect reason InvalidGenerateAddr, get %s", reason)
	}
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition != nil {
		t.Fatalf("expect nil condition, get %#v", ensureCondition)
	}
}

func TestBackendEnsure(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	//ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Status.BackendAddr = "fake.addr.com:1234"
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "SuccEnsureBackend" {
		t.Fatalf("expect reason SuccEnsureBackend, get %s", reason)
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition.Status != lbcfapi.ConditionTrue {
		t.Fatalf("expect condition.status=true, get %s", ensureCondition.Status)
	}
}

func TestBackendEnsureFailed(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	//ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Status.BackendAddr = "fake.addr.com:1234"
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect fail result, get %#v, err: %v", resp, resp.GetError())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "FailedEnsureBackend" {
		t.Fatalf("expect reason FailedEnsureBackend, get %s", reason)
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition.status=false, get %s", ensureCondition.Status)
	} else if ensureCondition.Reason != string(lbcfapi.ReasonOperationFailed) {
		t.Fatalf("expect condition.status %v, get %s", lbcfapi.ReasonOperationFailed, ensureCondition.Reason)
	} else if ensureCondition.Message == "" {
		t.Fatalf("expect non empty message")
	}
}

func TestBackendEnsureRunning(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyAlways,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetError())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "CalledEnsureBackend" {
		t.Fatalf("expect reason CalledEnsureBackend, get %s", reason)
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); !reflect.DeepEqual(*get, ensureCondition) {
		t.Errorf("expect condition not changed, get %v", get)
	}
}

func TestBackendEnsureNoRerun(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	if len(store) != 0 {
		t.Fatalf("expect 0 event, get %d", len(store))
	}

	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); get == nil {
		t.Fatalf("missing condition")
	} else if !reflect.DeepEqual(*get, ensureCondition) {
		t.Fatalf("expect condition %#v, get %#v", ensureCondition, *get)
	}
}

func TestBackendEnsureInvalidResponse(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	//ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Status.BackendAddr = "fake.addr.com:1234"
	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsError() {
		t.Fatalf("expect error result, get %#v, err: %v", resp, resp.GetError())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "InvalidEnsureBackend" {
		t.Fatalf("expect reason InvalidEnsureBackend, get %s", reason)
	}
}

func TestBackendDeregister(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.DeletionTimestamp = &ts
	backend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 0 {
		t.Fatalf("expect empty finalizer, get %#v", get.Finalizers)
	}
}

func TestBackendDeregisterFailed(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.DeletionTimestamp = &ts
	backend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect fail result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("get %#v", get.Finalizers)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "FailedDeregister" {
		t.Fatalf("expect reason FailedDeregister, get %s", reason)
	}
}

func TestBackendDeregisterRunning(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.DeletionTimestamp = &ts
	backend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("get %#v", get.Finalizers)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "CalledDeregister" {
		t.Fatalf("expect reason CalledDeregister, get %s", reason)
	}
}

func TestBackendDeregisterInvalidResponse(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.DeletionTimestamp = &ts
	backend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsError() {
		t.Fatalf("expect error result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("get %#v", get.Finalizers)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "InvalidDeregister" {
		t.Fatalf("expect reason InvalidDeregister, get %s", reason)
	}
}

func TestBackendDeregisterEmptyAddr(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.DeletionTimestamp = &ts
	backend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
	store := make(map[string]string)
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{
			get: backend,
		},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 0 {
		t.Fatalf("expect empty finalizer, get %#v", get.Finalizers)
	}
}
