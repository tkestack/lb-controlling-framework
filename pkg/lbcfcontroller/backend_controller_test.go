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
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
	"time"

	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/kubernetes/pkg/controller"
)

func TestBackendGenerateAddr(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
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

func TestBackendGenerateSvcAddr(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	svc := newFakeService("", "test-svc", v12.ServiceTypeNodePort)
	node := newFakeNode("", "node")
	bg := newFakeBackendGroupOfService("", "bg", lb.Name, 80, "TCP", svc.Name)
	backend := util.ConstructServiceBackendRecord(lb, bg, svc, node)
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
		&fakePodLister{},
		&fakeSvcListerWithStore{
			store: map[string]*v12.Service{
				svc.Name: svc,
			},
		},
		&fakeNodeListerWithStore{
			store: map[string]*v12.Node{
				node.Name: node,
			},
		},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
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

func TestBackendGenerateStaticAddr(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	staticAddr := "addr.com"
	bg := newFakeBackendGroupOfStatic("", "bg", lb.Name, staticAddr)
	backend := util.ConstructStaticBackend(lb, bg, staticAddr)
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
		&fakePodLister{},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect failed result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr != "" {
		t.Fatalf("expect empty addr, get %v", get.Status.BackendAddr)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "RunningGenerateAddr" {
		t.Fatalf("expect reason RunningGenerateAddr, get %s", reason)
	}
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition != nil {
		t.Fatalf("expect nil condition, get %#v", ensureCondition)
	}
}

func TestBackendGenerateAddrInvalidResponse(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect error result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect fail result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "RunningEnsureBackend" {
		t.Fatalf("expect reason RunningEnsureBackend, get %s", reason)
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); !reflect.DeepEqual(*get, ensureCondition) {
		t.Errorf("expect condition not changed, get %v", get)
	}
}

func TestBackendEnsureRerun(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	}

	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); get == nil {
		t.Fatalf("missing condition")
	} else if get.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition false, get %v", get.Status)
	}
}

func TestBackendEnsureInvalidResponse(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	//ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect error result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect fail result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("get %#v", get.Finalizers)
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[backend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", backend.Name, store)
	} else if reason != "RunningDeregister" {
		t.Fatalf("expect reason RunningDeregister, get %s", reason)
	}
}

func TestBackendDeregisterInvalidResponse(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeInvalidInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect error result, get %#v, err: %v", resp, resp.GetFailReason())
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
	backend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
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
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 0 {
		t.Fatalf("expect empty finalizer, get %#v", get.Finalizers)
	}
}

func TestBackendSameAddrOperation(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}

	// oldBackend is deleting
	oldBackend := util.ConstructPodBackendRecord(lb, bg, newFakePod("", "pod-0", nil, true, false))
	oldBackend.DeletionTimestamp = &ts
	oldBackend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	oldBackend.Spec.LBInfo = map[string]string{
		"lbID": "1234",
	}
	oldBackend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	oldBackend.Status.BackendAddr = "fake.addr.com:1234"
	oldBackend.Status.Conditions = []lbcfapi.BackendRecordCondition{
		{
			Type:               lbcfapi.BackendRegistered,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: ts,
		},
	}

	// newBackend has the same backendAddr and lbInfo
	pod2 := newFakePod("", "pod-0", nil, true, false)
	pod2.UID = "anotherUID"
	newBackend := util.ConstructPodBackendRecord(lb, bg, pod2)
	newBackend.Finalizers = []string{lbcfapi.FinalizerDeregisterBackend}
	newBackend.Spec.LBInfo = map[string]string{
		"lbID": "1234",
	}
	newBackend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	newBackend.Status.BackendAddr = "fake.addr.com:1234"

	fakeClient := fake.NewSimpleClientset(oldBackend, newBackend)
	store := make(map[string]string)
	backendLister := newFakeBackendListerWithStore()
	backendLister.store[oldBackend.Name] = oldBackend
	backendLister.store[newBackend.Name] = newBackend
	ctrl := newBackendController(
		fakeClient,
		backendLister,
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		&fakeEventRecorder{store: store},
		&fakeRunningInvoker{})

	key, _ := controller.KeyFunc(oldBackend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[oldBackend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", oldBackend.Name, store)
	} else if reason != "RunningDeregister" {
		t.Fatalf("expect reason RunningDeregister, get %s", reason)
	}
	delete(store, oldBackend.Name)

	// ensureBackend on newBackend should be delayed
	key, _ = controller.KeyFunc(newBackend)
	resp = ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[newBackend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", newBackend.Name, store)
	} else if reason != "DelayedEnsureBackend" {
		t.Fatalf("expect reason DelayedEnsureBackend, get %s", reason)
	}
	delete(store, newBackend.Name)

	// bypass oldBackend deregisterBackend webhook
	ctrl.webhookInvoker = &fakeSuccInvoker{}
	key, _ = controller.KeyFunc(oldBackend)
	resp = ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	delete(store, oldBackend.Name)

	// once oldBackend finished, ensureBackend of newBackend starts
	key, _ = controller.KeyFunc(newBackend)
	resp = ctrl.syncBackendRecord(key)
	if !resp.IsFinished() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetFailReason())
	}
	if len(store) != 1 {
		t.Fatalf("expect 1 event, get %d", len(store))
	} else if reason, ok := store[newBackend.Name]; !ok {
		t.Fatalf("expect event for %s, get %v", newBackend.Name, store)
	} else if reason != "SuccEnsureBackend" {
		t.Fatalf("expect reason SuccEnsureBackend, get %s", reason)
	}
	delete(store, newBackend.Name)
}
