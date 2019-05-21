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
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strings"
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
	addrCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated)
	if addrCondition.Status != lbcfapi.ConditionTrue {
		t.Fatalf("expect condition.status=true, get %s", addrCondition.Status)
	}
}

func TestBackendGenerateAddrFailed(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	fakeClient := fake.NewSimpleClientset(backend)
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
	addrCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated)
	if addrCondition.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition.status=false, get %s", addrCondition.Status)
	} else if addrCondition.Reason != string(lbcfapi.ReasonOperationFailed) {
		t.Fatalf("expect condition.reason=%v, get %s", lbcfapi.ReasonOperationFailed, addrCondition.Status)
	} else if addrCondition.Message == "" {
		t.Fatalf("expect condition.message not empty")
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
	addrCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated)
	if addrCondition.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition.status=false, get %s", addrCondition.Status)
	} else if addrCondition.Reason != string(lbcfapi.ReasonOperationInProgress) {
		t.Fatalf("expect condition.reason=%v, get %s", lbcfapi.ReasonOperationInProgress, addrCondition.Status)
	} else if addrCondition.Message == "" {
		t.Fatalf("expect condition.message not empty")
	}
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition != nil {
		t.Fatalf("expect nil condition, get %#v", ensureCondition)
	}
}

func TestBackendEnsure(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Status.BackendAddr = "fake.addr.com:1234"
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition}
	fakeClient := fake.NewSimpleClientset(backend)
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
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated); get == nil {
		t.Fatalf("missing condition")
	} else if !reflect.DeepEqual(*get, addrCondition) {
		t.Fatalf("expect condition %#v, get %#v", addrCondition, *get)
	}
	ensureCondition := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered)
	if ensureCondition.Status != lbcfapi.ConditionTrue {
		t.Fatalf("expect condition.status=true, get %s", ensureCondition.Status)
	}
}

func TestBackendEnsureFailed(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.Status.BackendAddr = "fake.addr.com:1234"
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition}
	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect fail result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr == "" {
		t.Fatalf("expect addr not empty")
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated); get == nil {
		t.Fatalf("missing condition")
	} else if !reflect.DeepEqual(*get, addrCondition) {
		t.Fatalf("expect condition %#v, get %#v", addrCondition, *get)
	}
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
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition, ensureCondition}
	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr == "" {
		t.Fatalf("expect addr not empty")
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated); get == nil {
		t.Fatalf("missing condition")
	} else if !reflect.DeepEqual(*get, addrCondition) {
		t.Fatalf("expect condition %#v, get %#v", addrCondition, *get)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); get.Status != ensureCondition.Status {
		t.Fatalf("expect condition.status %v, get %v", ensureCondition.Status, get.Status)
	} else if get.Reason != string(lbcfapi.ReasonOperationInProgress) {
		t.Fatalf("expect condition.status %v, get %s", lbcfapi.ReasonOperationInProgress, ensureCondition.Reason)
	} else if get.Message == "" {
		t.Fatalf("expect non empty message")
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
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition, ensureCondition}
	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get.Status.BackendAddr == "" {
		t.Fatalf("expect addr not empty")
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendAddrGenerated); get == nil {
		t.Fatalf("missing condition")
	} else if !reflect.DeepEqual(*get, addrCondition) {
		t.Fatalf("expect condition %#v, get %#v", addrCondition, *get)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); get == nil {
		t.Fatalf("missing condition")
	} else if !reflect.DeepEqual(*get, ensureCondition) {
		t.Fatalf("expect condition %#v, get %#v", ensureCondition, *get)
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
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition, ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeSuccInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %v, get %#v", lbcfapi.FinalizerDeregisterBackend, get.Finalizers)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); get.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition.status %v, get %v", lbcfapi.ConditionFalse, get.Status)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendReadyToDelete); get.Status != lbcfapi.ConditionTrue {
		t.Fatalf("expect condition.status %v, get %v", lbcfapi.ConditionTrue, get.Status)
	}
}

func TestBackendDeregisterSkipped(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	ts := v1.Time{time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	backend.DeletionTimestamp = &ts
	backend.Finalizers = []string{}
	backend.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyIfNotSucc,
	}
	backend.Status.BackendAddr = "fake.addr.com:1234"
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionFalse,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition, ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
}

func TestBackendRemoveFinalizer(t *testing.T) {
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
	readyToDelete := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendReadyToDelete,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{readyToDelete}

	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeFailInvoker{})
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

func TestBackendFailed(t *testing.T) {
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
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition, ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsFailed() {
		t.Fatalf("expect fail result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %v, get %#v", lbcfapi.FinalizerDeregisterBackend, get.Finalizers)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); !reflect.DeepEqual(*get, ensureCondition) {
		t.Fatalf("expect condition.status %v, get %v", lbcfapi.ConditionFalse, get.Status)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendReadyToDelete); get.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition.status %v, get %v", lbcfapi.ConditionFalse, get.Status)
	} else if get.Reason != lbcfapi.ReasonOperationFailed.String() {
		t.Fatalf("expect reason %v, get %v", lbcfapi.ReasonOperationFailed, get.Reason)
	} else if get.Message == "" {
		t.Fatalf("expect message")
	}
}

func TestBackendRunning(t *testing.T) {
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
	addrCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendAddrGenerated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	ensureCondition := lbcfapi.BackendRecordCondition{
		Type:               lbcfapi.BackendRegistered,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	}
	backend.Status.Conditions = []lbcfapi.BackendRecordCondition{addrCondition, ensureCondition}

	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeRunningInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsRunning() {
		t.Fatalf("expect running result, get %#v, err: %v", resp, resp.GetError())
	}
	get, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if len(get.Finalizers) != 1 {
		t.Fatalf("expect finalizer %v, get %#v", lbcfapi.FinalizerDeregisterBackend, get.Finalizers)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendRegistered); !reflect.DeepEqual(*get, ensureCondition) {
		t.Fatalf("expect condition.status %v, get %v", lbcfapi.ConditionFalse, get.Status)
	}
	if get := util.GetBackendRecordCondition(&get.Status, lbcfapi.BackendReadyToDelete); get.Status != lbcfapi.ConditionFalse {
		t.Fatalf("expect condition.status %v, get %v", lbcfapi.ConditionFalse, get.Status)
	} else if get.Reason != lbcfapi.ReasonOperationInProgress.String() {
		t.Fatalf("expect reason %v, get %v", lbcfapi.ReasonOperationInProgress, get.Reason)
	} else if get.Message == "" {
		t.Fatalf("expect message")
	}
}

func TestBackendSetInvalidOperation(t *testing.T) {
	backend := newFakeBackendRecord("", "backend")
	fakeClient := fake.NewSimpleClientset(backend)
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
		&fakeRunningInvoker{})
	resp := webhooks.ResponseForFailRetryHooks{
		Status: "invalidStatus",
		Msg:    "fake message",
	}
	result := ctrl.setOperationInvalidResponse(backend, resp, lbcfapi.BackendAddrGenerated)
	if !result.IsError() {
		t.Fatalf("expect err result, get %#v", result)
	}
	obj, _ := fakeClient.LbcfV1beta1().BackendRecords(backend.Namespace).Get(backend.Name, v1.GetOptions{})
	if get := util.GetBackendRecordCondition(&obj.Status, lbcfapi.BackendAddrGenerated); get == nil {
		t.Fatalf("missing condition")
	} else if get.Reason != lbcfapi.ReasonInvalidResponse.String() {
		t.Fatalf("expect reason %v, get %v", lbcfapi.ReasonInvalidResponse, get.Reason)
	} else if index := strings.Index(get.Message, "unknown status"); index == -1 {
		t.Fatalf("wrong message, get %v", get.Message)
	}
}

func TestBackendNotFoundBackend(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods("", "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	backend := util.ConstructBackendRecord(lb, bg, "pod-0")
	fakeClient := fake.NewSimpleClientset()
	ctrl := newBackendController(
		fakeClient,
		&fakeBackendLister{},
		&fakeDriverLister{
			get: newFakeDriver("", "driver"),
		},
		&fakePodLister{
			get: newFakePod("", "pod=0", nil, true, false),
		},
		&fakeFailInvoker{})
	key, _ := controller.KeyFunc(backend)
	resp := ctrl.syncBackendRecord(key)
	if !resp.IsSucc() {
		t.Fatalf("expect succ result, get %#v, err: %v", resp, resp.GetError())
	}
}
