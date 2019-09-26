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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller"
	"strings"
	"testing"
	"time"

	lbcfapi "git.code.oa.com/tkestack/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.code.oa.com/tkestack/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	lbcflister "git.code.oa.com/tkestack/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.code.oa.com/tkestack/lb-controlling-framework/pkg/lbcfcontroller/util"
	"git.code.oa.com/tkestack/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/listers/core/v1"
)

func TestLBCFControllerAddPod(t *testing.T) {
	podLabel := map[string]string{
		"k1": "v1",
	}
	pod1 := newFakePod("", "pod-1", podLabel, true, false)
	bg1 := newFakeBackendGroupOfPods("", "bg-1", "", 80, "tcp", podLabel, nil, nil)

	pod2 := newFakePod("", "pod-2", nil, true, false)
	bg2 := newFakeBackendGroupOfPods("", "bg-2", "", 80, "tcp", nil, nil, []string{"pod-2"})
	bgCtrl := newBackendGroupController(
		fake.NewSimpleClientset(),
		&fakeLBLister{},
		&fakeBackendGroupLister{
			list: []*lbcfapi.BackendGroup{bg1, bg2},
		},
		&fakeBackendLister{},
		&fakePodLister{},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
	)
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.addPod(pod1)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.addPod(pod2)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg2); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerUpdatePod_PodStatusChange(t *testing.T) {
	podLabel := map[string]string{
		"k1": "v1",
	}
	oldPod1 := newFakePod("", "pod-0", podLabel, false, false)
	curPod1 := newFakePod("", "pod-0", podLabel, true, false)
	curPod1.ResourceVersion = "another"

	oldPod2 := newFakePod("", "pod-0", podLabel, true, false)
	curPod2 := newFakePod("", "pod-0", podLabel, false, false)
	curPod2.ResourceVersion = "another"

	oldPod3 := newFakePod("", "pod-0", podLabel, true, false)
	curPod3 := newFakePod("", "pod-0", podLabel, false, true)
	curPod3.ResourceVersion = "another"

	bg1 := newFakeBackendGroupOfPods("", "bg-0", "", 80, "tcp", podLabel, nil, nil)

	bgCtrl := newBackendGroupController(
		fake.NewSimpleClientset(),
		&fakeLBLister{},
		&fakeBackendGroupLister{
			list: []*lbcfapi.BackendGroup{bg1},
		},
		&fakeBackendLister{},
		&fakePodLister{},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
	)
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.updatePod(oldPod1, curPod1)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.updatePod(oldPod2, curPod2)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.updatePod(oldPod3, curPod3)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerUpdatePod_PodLabelChange(t *testing.T) {
	podLabel1 := map[string]string{
		"k1": "v1",
	}
	podLabel1Plus := map[string]string{
		"k1":       "v1",
		"addition": "value",
	}
	podLabel2 := map[string]string{
		"k2": "v2",
	}
	oldPod1 := newFakePod("", "pod-1", nil, true, false)
	curPod1 := newFakePod("", "pod-1", podLabel1, true, false)
	curPod1.ResourceVersion = "another"

	oldPod2 := newFakePod("", "pod-2", podLabel1, true, false)
	curPod2 := newFakePod("", "pod-2", nil, true, false)
	curPod2.ResourceVersion = "another"

	oldPod3 := newFakePod("", "pod-3", podLabel1, true, false)
	curPod3 := newFakePod("", "pod-3", podLabel2, true, false)
	curPod3.ResourceVersion = "another"

	oldPod4 := newFakePod("", "pod-3", podLabel1, true, false)
	curPod4 := newFakePod("", "pod-3", podLabel1Plus, true, false)
	curPod4.ResourceVersion = "another"

	bg1 := newFakeBackendGroupOfPods("", "bg-1", "", 80, "tcp", podLabel1, nil, nil)
	bg2 := newFakeBackendGroupOfPods("", "bg-2", "", 80, "tcp", podLabel2, nil, nil)

	bgCtrl := newBackendGroupController(
		fake.NewSimpleClientset(),
		&fakeLBLister{},
		&fakeBackendGroupLister{
			list: []*lbcfapi.BackendGroup{bg1, bg2},
		},
		&fakeBackendLister{},
		&fakePodLister{},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
	)
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.updatePod(oldPod1, curPod1)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.updatePod(oldPod2, curPod2)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.updatePod(oldPod3, curPod3)
	if c.backendGroupQueue.Len() != 2 {
		t.Fatalf("queue length should be 2, get %d", c.backendGroupQueue.Len())
	}
	getKeySet := sets.NewString()
	key1, done := c.backendGroupQueue.Get()
	if key1 == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key1.(string); !ok {
		t.Error("key is not a string")
	} else {
		getKeySet.Insert(key)
	}
	key2, done := c.backendGroupQueue.Get()
	if key2 == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key2.(string); !ok {
		t.Error("key is not a string")
	} else {
		getKeySet.Insert(key)
	}
	expectKey1, _ := controller.KeyFunc(bg1)
	expectKey2, _ := controller.KeyFunc(bg2)
	if !getKeySet.Has(expectKey1) {
		t.Errorf("miss BackendGroup key %s", expectKey1)
	}
	if !getKeySet.Has(expectKey2) {
		t.Errorf("miss BackendGroup key %s", expectKey2)
	}
	c.backendGroupQueue.Done(key1)
	c.backendGroupQueue.Done(key2)

	c.updatePod(oldPod4, curPod4)
	if c.backendGroupQueue.Len() != 0 {
		t.Fatalf("queue length should be 0, get %d", c.backendGroupQueue.Len())
	}
}

func TestLBCFControllerDeletePod(t *testing.T) {
	podLabel1 := map[string]string{
		"k1": "v1",
	}
	pod1 := newFakePod("", "pod-1", podLabel1, false, false)
	bg1 := newFakeBackendGroupOfPods("", "bg-1", "", 80, "tcp", podLabel1, nil, nil)
	tomestoneKey, _ := controller.KeyFunc(pod1)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: pod1}

	bgCtrl := newBackendGroupController(
		fake.NewSimpleClientset(),
		&fakeLBLister{},
		&fakeBackendGroupLister{
			list: []*lbcfapi.BackendGroup{bg1},
		},
		&fakeBackendLister{},
		&fakePodLister{},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
	)
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.deletePod(pod1)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.deletePod(tombstone)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg1); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerAddBackendGroup(t *testing.T) {
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)
	bg := newFakeBackendGroupOfPods("", "bg", "", 80, "tcp", nil, nil, nil)
	c.addBackendGroup(bg)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerUpdateBackendGroup(t *testing.T) {
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)
	oldGroup := newFakeBackendGroupOfPods("", "bg", "", 80, "tcp", nil, nil, nil)
	curGroup := newFakeBackendGroupOfPods("", "bg", "", 80, "tcp", nil, nil, nil)

	c.updateBackendGroup(oldGroup, curGroup)
	if c.backendGroupQueue.Len() != 0 {
		t.Fatalf("queue length should be 0, get %d", c.backendGroupQueue.Len())
	}

	curGroup.ResourceVersion = "2"
	c.updateBackendGroup(oldGroup, curGroup)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(curGroup); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerDeleteBackendGroup(t *testing.T) {
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)
	bg := newFakeBackendGroupOfPods("", "bg", "", 80, "tcp", nil, nil, nil)
	c.deleteBackendGroup(bg)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	tomestoneKey, _ := controller.KeyFunc(bg)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: bg}
	c.deleteBackendGroup(tombstone)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerAddService(t *testing.T) {
	svc := newFakeService("", "test-svc", apiv1.ServiceTypeNodePort)
	bg := newFakeBackendGroupOfService(svc.Namespace, "bg", "lb", 80, "TCP", svc.Name)
	bg2 := newFakeBackendGroupOfService(svc.Namespace, "another-bg", "lb", 80, "TCP", "another-svc")

	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.addService(svc)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}

	groupKey, done := c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)
}

func TestLBCFControllerUpdateService(t *testing.T) {
	oldSvc := newFakeService("", "test-svc", apiv1.ServiceTypeNodePort)
	statusChangedSvc := *oldSvc
	statusChangedSvc.ResourceVersion = "another-rv"

	bg := newFakeBackendGroupOfService(oldSvc.Namespace, "bg", "lb", 80, "TCP", oldSvc.Name)
	bg2 := newFakeBackendGroupOfService(oldSvc.Namespace, "another-bg", "lb", 80, "TCP", "another-svc")

	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.updateService(oldSvc, &statusChangedSvc)
	if c.backendGroupQueue.Len() != 0 {
		t.Fatalf("queue length should be 0, get %d", c.backendGroupQueue.Len())
	}

	specChangedSvc := statusChangedSvc
	specChangedSvc.Generation++

	c.updateService(oldSvc, &specChangedSvc)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}

	groupKey, done := c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)
}

func TestLBCFControllerDeleteService(t *testing.T) {
	svc := newFakeService("", "test-svc", apiv1.ServiceTypeNodePort)
	bg := newFakeBackendGroupOfService(svc.Namespace, "bg", "lb", 80, "TCP", svc.Name)
	bg2 := newFakeBackendGroupOfService(svc.Namespace, "another-bg", "lb", 80, "TCP", "another-svc")
	tomestoneKey, _ := controller.KeyFunc(bg)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: svc}

	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, nil, nil, bgCtrl)

	c.deleteService(svc)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}

	groupKey, done := c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)

	c.deleteService(tombstone)
	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}

	groupKey, done = c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)
}

func TestLBCFControllerAddLoadBalancer(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods(lb.Namespace, "bg", lb.Name, 80, "tcp", nil, nil, nil)
	bg2 := newFakeBackendGroupOfPods(lb.Namespace, "bg", "another-lb", 80, "tcp", nil, nil, nil)

	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, lbCtrl, nil, bgCtrl)

	c.addLoadBalancer(lb)
	if c.loadBalancerQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.loadBalancerQueue.Len())
	} else if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}

	lbKey, done := c.loadBalancerQueue.Get()
	if lbKey == nil || done {
		t.Error("failed to enqueue LoadBalancer")
	} else if key, ok := lbKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(lb); expectedKey != key {
		t.Errorf("expected LoadBalancer key %s found %s", expectedKey, key)
	}
	c.loadBalancerQueue.Done(lbKey)

	groupKey, done := c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)
}

func TestLBCFControllerUpdateLoadBalancer(t *testing.T) {
	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bg := newFakeBackendGroupOfPods("", "bg", "lb", 80, "TCP", nil, nil, nil)
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	type testCase struct {
		name          string
		old           *lbcfapi.LoadBalancer
		cur           *lbcfapi.LoadBalancer
		ctrl          *Controller
		expectLBQueue int
		expectBGQueue int
	}
	cases := []testCase{
		{
			name:          "periodic-resync",
			old:           newFakeLoadBalancer("", "lb", nil, nil),
			cur:           newFakeLoadBalancer("", "lb", nil, nil),
			ctrl:          newFakeLBCFController(nil, lbCtrl, nil, bgCtrl),
			expectLBQueue: 0,
			expectBGQueue: 0,
		},
		{
			name: "update-attr",
			old: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Spec.Attributes = map[string]string{
					"a1": "v1",
				}
				return lb
			}(),
			cur: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Spec.Attributes = map[string]string{
					"a1": "v2",
				}
				lb.ResourceVersion = "another"
				lb.Generation = 2
				return lb
			}(),
			ctrl:          newFakeLBCFController(nil, lbCtrl, nil, bgCtrl),
			expectLBQueue: 1,
			expectBGQueue: 1,
		},
		{
			name: "create-succ-but-not-sync",
			old: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				return lb
			}(),
			cur: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
				}
				lb.ResourceVersion = "another"
				return lb
			}(),
			ctrl:          newFakeLBCFController(nil, lbCtrl, nil, bgCtrl),
			expectLBQueue: 1,
			expectBGQueue: 1,
		},
		{
			name: "sync-succ",
			old: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				return lb
			}(),
			cur: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
					{
						Type:   lbcfapi.LBAttributesSynced,
						Status: lbcfapi.ConditionTrue,
					},
				}
				lb.ResourceVersion = "another"
				return lb
			}(),
			ctrl:          newFakeLBCFController(nil, lbCtrl, nil, bgCtrl),
			expectLBQueue: 0,
			expectBGQueue: 1,
		},
		{
			name: "ensure-failed",
			old: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
					{
						Type:   lbcfapi.LBAttributesSynced,
						Status: lbcfapi.ConditionTrue,
					},
				}
				return lb
			}(),
			cur: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
					{
						Type:    lbcfapi.LBAttributesSynced,
						Status:  lbcfapi.ConditionFalse,
						Message: "some message",
					},
				}
				lb.ResourceVersion = "another"
				return lb
			}(),
			ctrl:          newFakeLBCFController(nil, lbCtrl, nil, bgCtrl),
			expectLBQueue: 0,
			expectBGQueue: 1,
		},
		{
			name: "delete-lb",
			old: func() *lbcfapi.LoadBalancer {
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
					{
						Type:   lbcfapi.LBAttributesSynced,
						Status: lbcfapi.ConditionTrue,
					},
				}
				return lb
			}(),
			cur: func() *lbcfapi.LoadBalancer {
				dt := metav1.Now()
				lb := newFakeLoadBalancer("", "lb", nil, nil)
				lb.Status.Conditions = []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
					{
						Type:   lbcfapi.LBAttributesSynced,
						Status: lbcfapi.ConditionTrue,
					},
				}
				lb.DeletionTimestamp = &dt
				lb.ResourceVersion = "another"
				return lb
			}(),
			ctrl:          newFakeLBCFController(nil, lbCtrl, nil, bgCtrl),
			expectLBQueue: 1,
			expectBGQueue: 1,
		},
	}

	for _, c := range cases {
		c.ctrl.updateLoadBalancer(c.old, c.cur)
		if c.ctrl.loadBalancerQueue.Len() != c.expectLBQueue {
			t.Errorf("case %s, expect %d, get %d", c.name, c.expectLBQueue, c.ctrl.loadBalancerQueue.Len())
			continue
		} else if c.ctrl.backendGroupQueue.Len() != c.expectBGQueue {
			t.Errorf("case %s, expect %d, get %d", c.name, c.expectBGQueue, c.ctrl.backendGroupQueue.Len())
			continue
		}

		if c.expectLBQueue > 0 {
			key, done := c.ctrl.loadBalancerQueue.Get()
			if key == nil || done {
				t.Error("failed to enqueue BackendGroup")
			} else if key, ok := key.(string); !ok {
				t.Error("key is not a string")
			} else if expectedKey, _ := controller.KeyFunc(c.cur); expectedKey != key {
				t.Errorf("expected LoadBalancer key %s found %s", expectedKey, key)
			}
			c.ctrl.loadBalancerQueue.Done(key)
		}

		if c.expectBGQueue > 0 {
			key, done := c.ctrl.backendGroupQueue.Get()
			if key == nil || done {
				t.Error("failed to enqueue BackendGroup")
			} else if key, ok := key.(string); !ok {
				t.Error("key is not a string")
			} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
				t.Errorf("expected LoadBalancer key %s found %s", expectedKey, key)
			}
			c.ctrl.loadBalancerQueue.Done(key)
		}
	}
}

func TestLBCFControllerDeleteLoadBalancer(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods(lb.Namespace, "bg", lb.Name, 80, "tcp", nil, nil, nil)
	bg2 := newFakeBackendGroupOfPods(lb.Namespace, "bg", "another-lb", 80, "tcp", nil, nil, nil)
	tomestoneKey, _ := controller.KeyFunc(bg)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: lb}

	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	c := newFakeLBCFController(nil, lbCtrl, nil, bgCtrl)

	c.deleteLoadBalancer(lb)
	if c.loadBalancerQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.loadBalancerQueue.Len())
	} else if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}

	lbKey, done := c.loadBalancerQueue.Get()
	if lbKey == nil || done {
		t.Error("failed to enqueue LoadBalancer")
	} else if key, ok := lbKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(lb); expectedKey != key {
		t.Errorf("expected LoadBalancer key %s found %s", expectedKey, key)
	}
	c.loadBalancerQueue.Done(lbKey)

	groupKey, done := c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)

	c.deleteLoadBalancer(tombstone)
	if c.loadBalancerQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.loadBalancerQueue.Len())
	} else if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	lbKey, done = c.loadBalancerQueue.Get()
	if lbKey == nil || done {
		t.Error("failed to enqueue LoadBalancer")
	} else if key, ok := lbKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(lb); expectedKey != key {
		t.Errorf("expected LoadBalancer key %s found %s", expectedKey, key)
	}
	c.loadBalancerQueue.Done(lbKey)

	groupKey, done = c.backendGroupQueue.Get()
	if groupKey == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := groupKey.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(groupKey)
}

func TestLBCFControllerAddDriver(t *testing.T) {
	driver := newFakeDriver("", "driver")
	driverCtrl := newDriverController(fake.NewSimpleClientset(), &fakeDriverLister{})
	c := newFakeLBCFController(driverCtrl, nil, nil, nil)

	c.addLoadBalancerDriver(driver)
	if c.driverQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.driverQueue.Len())
	}
	key, done := c.driverQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(driver); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.driverQueue.Done(key)
}

func TestLBCFControllerUpdateDriver(t *testing.T) {
	oldDriver := newFakeDriver("", "driver")
	curDriver := newFakeDriver("", "driver")
	driverCtrl := newDriverController(fake.NewSimpleClientset(), &fakeDriverLister{})
	c := newFakeLBCFController(driverCtrl, nil, nil, nil)

	c.updateLoadBalancerDriver(oldDriver, curDriver)
	if c.driverQueue.Len() != 0 {
		t.Fatalf("queue length should be 0, get %d", c.driverQueue.Len())
	}

	curDriver.ResourceVersion = "2"
	c.updateLoadBalancerDriver(oldDriver, curDriver)
	if c.driverQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.driverQueue.Len())
	}
	key, done := c.driverQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(curDriver); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.driverQueue.Done(key)
}

func TestLBCFControllerDeleteDriver(t *testing.T) {
	driver := newFakeDriver("", "driver")
	driverCtrl := newDriverController(fake.NewSimpleClientset(), &fakeDriverLister{})
	tomestoneKey, _ := controller.KeyFunc(driver)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: driver}

	c := newFakeLBCFController(driverCtrl, nil, nil, nil)

	c.deleteLoadBalancerDriver(driver)
	if c.driverQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.driverQueue.Len())
	}
	key, done := c.driverQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(driver); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.driverQueue.Done(key)

	c.deleteLoadBalancerDriver(tombstone)
	if c.driverQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.driverQueue.Len())
	}
	key, done = c.driverQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(driver); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.driverQueue.Done(key)
}

func TestLBCFControllerAddBackendRecord(t *testing.T) {
	record := newFakeBackendRecord("", "record")
	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	c := newFakeLBCFController(nil, nil, backendCtrl, nil)

	c.addBackendRecord(record)
	if c.backendQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendQueue.Len())
	}
	key, done := c.backendQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(record); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendQueue.Done(key)
}

func TestLBCFControllerUpdateBackendRecord(t *testing.T) {
	type testCase struct {
		name          string
		old           *lbcfapi.BackendRecord
		cur           *lbcfapi.BackendRecord
		ctrl          *Controller
		expectQueue   int
		expectBGQueue int
	}
	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bg := newFakeBackendGroupOfPods("", "bg", "lb", 80, "TCP", nil, nil, nil)
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg},
	}, &fakeBackendLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{})
	cases := []testCase{
		{
			name: "periodic-resync",
			old: func() *lbcfapi.BackendRecord {
				backend := newFakeBackendRecord("", "record")
				setBackendRecordOwner(backend, bg)
				return backend
			}(),
			cur: func() *lbcfapi.BackendRecord {
				backend := newFakeBackendRecord("", "record")
				setBackendRecordOwner(backend, bg)
				return backend
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   0,
			expectBGQueue: 0,
		},
		{
			name: "set-backendaddr",
			old: func() *lbcfapi.BackendRecord {
				backend := newFakeBackendRecord("", "record")
				setBackendRecordOwner(backend, bg)
				return backend
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.ResourceVersion = "another-version"
				record.Status.BackendAddr = "fakeaddr.com"
				setBackendRecordOwner(record, bg)
				return record
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   1,
			expectBGQueue: 0,
		},
		{
			name: "ensure-succ",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				setBackendRecordOwner(record, bg)
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.ResourceVersion = "2"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionTrue,
					},
				}
				setBackendRecordOwner(record, bg)
				return record
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   0,
			expectBGQueue: 1,
		},
		{
			name: "ensure-fail",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				setBackendRecordOwner(record, bg)
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.ResourceVersion = "another-rv"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionFalse,
					},
				}
				setBackendRecordOwner(record, bg)
				return record
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   0,
			expectBGQueue: 0,
		},
		{
			name: "re-ensure-fail",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionTrue,
					},
				}
				setBackendRecordOwner(record, bg)
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.ResourceVersion = "another-rv"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:    lbcfapi.BackendRegistered,
						Status:  lbcfapi.ConditionFalse,
						Message: "some message",
					},
				}
				setBackendRecordOwner(record, bg)
				return record
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   0,
			expectBGQueue: 1,
		},
		{
			name: "update-parameters",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionTrue,
					},
				}
				setBackendRecordOwner(record, bg)
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionTrue,
					},
				}
				record.Spec.Parameters = map[string]string{
					"newKey": "newValue",
				}
				record.ResourceVersion = "another-rv"
				record.Generation = 2
				setBackendRecordOwner(record, bg)
				return record
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   1,
			expectBGQueue: 0,
		},
		{
			name: "delete-backendrecord",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionTrue,
					},
				}
				setBackendRecordOwner(record, bg)
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				dt := metav1.Now()
				record := newFakeBackendRecord("", "record")
				record.DeletionTimestamp = &dt
				record.Status.BackendAddr = "fakeaddr.com"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionTrue,
					},
				}
				record.ResourceVersion = "another-rv"
				setBackendRecordOwner(record, bg)
				return record
			}(),
			ctrl:          newFakeLBCFController(nil, nil, backendCtrl, bgCtrl),
			expectQueue:   1,
			expectBGQueue: 0,
		},
	}
	for _, c := range cases {
		c.ctrl.updateBackendRecord(c.old, c.cur)
		if c.ctrl.backendQueue.Len() != c.expectQueue {
			t.Errorf("case %s, expect len %d, get %d", c.name, c.expectQueue, c.ctrl.backendQueue.Len())
			continue
		} else if c.ctrl.backendGroupQueue.Len() != c.expectBGQueue {
			t.Errorf("case %s, expect len %d, get %d", c.name, c.expectBGQueue, c.ctrl.backendGroupQueue.Len())
			continue
		}

		if c.expectQueue > 0 {
			key, done := c.ctrl.backendQueue.Get()
			if key == nil || done {
				t.Error("failed to enqueue BackendGroup")
			} else if key, ok := key.(string); !ok {
				t.Error("key is not a string")
			} else if expectedKey, _ := controller.KeyFunc(c.cur); expectedKey != key {
				t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
			}
			c.ctrl.backendQueue.Done(key)
			if c.ctrl.backendGroupQueue.Len() != 0 {
				t.Fatalf("expect empty backendGroup queue, get %v", c.ctrl.backendGroupQueue.Len())
			}
		}

		if c.expectBGQueue > 0 {
			key, done := c.ctrl.backendGroupQueue.Get()
			if key == nil || done {
				t.Error("failed to enqueue BackendGroup")
			} else if key, ok := key.(string); !ok {
				t.Error("key is not a string")
			} else if expectedKey, _ := controller.KeyFunc(bg); expectedKey != key {
				t.Errorf("expected BackendGroup key %s found %s", expectedKey, key)
			}
			c.ctrl.loadBalancerQueue.Done(key)
		}
	}
}

func TestLBCFControllerDeleteBackendRecord(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	group := newFakeBackendGroupOfPods(lb.Namespace, "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	record := util.ConstructPodBackendRecord(lb, group, newFakePod("", "pod-0", nil, true, false))
	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeSvcListerWithStore{}, &fakeNodeListerWithStore{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	tomestoneKey, _ := controller.KeyFunc(record)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: record}
	c := newFakeLBCFController(nil, nil, backendCtrl, nil)

	c.deleteBackendRecord(record)
	if c.backendQueue.Len() != 0 {
		t.Fatalf("queue length should be 0, get %d", c.backendQueue.Len())
	} else if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done := c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(group); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)

	c.deleteBackendRecord(tombstone)
	if c.backendQueue.Len() != 0 {
		t.Fatalf("queue length should be 0, get %d", c.backendQueue.Len())
	} else if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
	}
	key, done = c.backendGroupQueue.Get()
	if key == nil || done {
		t.Error("failed to enqueue BackendGroup")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(group); expectedKey != key {
		t.Errorf("expected Backendgroup key %s found %s", expectedKey, key)
	}
	c.backendGroupQueue.Done(key)
}

func TestLBCFControllerProcessNextItemSucc(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second)
	obj := newFakeDriver("", "driver")
	ctrl.enqueue(obj, q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.FinishedResult()
	})
	if q.Len() != 0 || q.LenWaitingForFilter() != 0 {
		t.Fatalf("expect 0, get %d, %d", q.Len(), q.LenWaitingForFilter())
	}
	//select {
	//case <-time.NewTimer(2 * time.Second).C:
	//case <-func() chan struct{} {
	//	ch := make(chan struct{})
	//	go func() {
	//		q.Get()
	//		close(ch)
	//	}()
	//	return ch
	//}():
	//	t.Fatalf("expect get nothing")
	//}
}

func TestLBCFControllerProcessNextItemError(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second)
	ctrl.enqueue("key", q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.ErrorResult(fmt.Errorf("fake error"))
	})
	if get, done := q.Get(); done {
		t.Fatalf("failed to get queue elements")
	} else if key, ok := get.(string); !ok {
		t.Fatalf("not a string")
	} else if key != "key" {
		t.Fatalf("expect key %q, get %q", "key", key)
	}
}

func TestLBCFControllerProcessNextItemFailed(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewConditionalDelayingQueue(nil, time.Millisecond, time.Millisecond, 2*time.Second)
	obj := newFakeDriver("", "driver")
	key, _ := controller.KeyFunc(obj)

	ctrl.enqueue(obj, q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.FailResult(500*time.Millisecond, "")
	})
	if get, done := q.Get(); done {
		t.Fatalf("failed to get queue elements")
	} else if get != key {
		t.Fatalf("expect key %q, get %q", key, get)
	}
}

func TestLBCFControllerProcessNextItemRunning(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second)
	obj := newFakeDriver("", "driver")
	key, _ := controller.KeyFunc(obj)

	ctrl.enqueue(obj, q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.AsyncResult(500 * time.Millisecond)
	})
	if get, done := q.Get(); done {
		t.Fatalf("failed to get queue elements")
	} else if get != key {
		t.Fatalf("expect key %q, get %q", key, get)
	}
}

func TestLBCFControllerProcessNextItemPeriodic(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second)
	obj := newFakeDriver("", "driver")
	key, _ := controller.KeyFunc(obj)

	ctrl.enqueue(obj, q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.PeriodicResult(500 * time.Millisecond)
	})
	if get, done := q.Get(); done {
		t.Fatalf("failed to get queue elements")
	} else if get != key {
		t.Fatalf("expect key %q, get %q", key, get)
	}
}

func newFakeBackendRecord(namespace, name string) *lbcfapi.BackendRecord {
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func setBackendRecordOwner(backend *lbcfapi.BackendRecord, group *lbcfapi.BackendGroup) {
	valueTrue := true
	backend.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         lbcfapi.ApiVersion,
			BlockOwnerDeletion: &valueTrue,
			Controller:         &valueTrue,
			Kind:               "BackendGroup",
			Name:               group.Name,
			UID:                group.UID,
		},
	}
}

func newFakeDriver(namespace, name string) *lbcfapi.LoadBalancerDriver {
	if strings.HasPrefix(name, lbcfapi.SystemDriverPrefix) {
		namespace = "kube-system"
	}
	return &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newFakeLoadBalancer(namespace, name string, attributes map[string]string, ensurePolicy *lbcfapi.EnsurePolicyConfig) *lbcfapi.LoadBalancer {
	return &lbcfapi.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: lbcfapi.LoadBalancerSpec{
			Attributes:   attributes,
			EnsurePolicy: ensurePolicy,
		},
	}
}

func newFakeBackendGroupOfPods(namespace, name string, lbName string, portNum int32, protocol string, labelSelector map[string]string, labelExcept []string, byName []string) *lbcfapi.BackendGroup {
	group := &lbcfapi.BackendGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: lbcfapi.BackendGroupSpec{
			LBName: lbName,
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: portNum,
					Protocol:   protocol,
				},
			},
		},
	}
	if len(labelSelector) > 0 {
		group.Spec.Pods.ByLabel = &lbcfapi.SelectPodByLabel{
			Selector: labelSelector,
			Except:   labelExcept,
		}
		return group
	}
	group.Spec.Pods.ByName = byName
	return group
}

func newFakeBackendGroupOfService(namespace, name string, lbName string, portNum int32, protocol string, serviceName string) *lbcfapi.BackendGroup {
	group := &lbcfapi.BackendGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: lbcfapi.BackendGroupSpec{
			LBName: lbName,
			Service: &lbcfapi.ServiceBackend{
				Name: serviceName,
				Port: lbcfapi.PortSelector{
					PortNumber: portNum,
					Protocol:   protocol,
				},
			},
		},
	}
	return group
}

func newFakeBackendGroupOfStatic(namespace, name string, lbName string, staticAddrs ...string) *lbcfapi.BackendGroup {
	group := &lbcfapi.BackendGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: lbcfapi.BackendGroupSpec{
			LBName: lbName,
			Static: staticAddrs,
		},
	}
	return group
}

func newFakePod(namespace string, name string, labels map[string]string, running bool, deleting bool) *apiv1.Pod {
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			UID:       "12345",
		},
	}
	if running && !deleting {
		pod.Status = apiv1.PodStatus{
			PodIP: "1.1.1.1",
			Conditions: []apiv1.PodCondition{
				{
					Type:   apiv1.PodReady,
					Status: apiv1.ConditionTrue,
				},
			},
		}
	}

	if deleting {
		ts := metav1.Now()
		pod.DeletionTimestamp = &ts
	}
	return pod
}

func newFakeService(namespace, name string, svcType apiv1.ServiceType) *apiv1.Service {
	svc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: apiv1.ServiceSpec{
			Type: svcType,
		},
	}
	port1 := apiv1.ServicePort{
		Name:       "http",
		Port:       80,
		Protocol:   apiv1.ProtocolTCP,
		TargetPort: intstr.FromInt(80),
	}
	port2 := apiv1.ServicePort{
		Name:       "https",
		Port:       443,
		Protocol:   apiv1.ProtocolTCP,
		TargetPort: intstr.FromInt(443),
	}
	if svcType == apiv1.ServiceTypeNodePort || svcType == apiv1.ServiceTypeLoadBalancer {
		port1.NodePort = 30000 + port1.Port
		port2.NodePort = 30000 + port2.Port
	}
	svc.Spec.Ports = []apiv1.ServicePort{port1, port2}
	return svc
}

func newFakeNode(namespace, name string) *apiv1.Node {
	return &apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newFakeLBCFController(driverCtrl *driverController, lbCtrl *loadBalancerController, backendCtrl *backendController, bgCtrl *backendGroupController) *Controller {
	return &Controller{
		driverCtrl:       driverCtrl,
		lbCtrl:           lbCtrl,
		backendCtrl:      backendCtrl,
		backendGroupCtrl: bgCtrl,

		driverQueue:       util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second),
		loadBalancerQueue: util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second),
		backendGroupQueue: util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second),
		backendQueue:      util.NewConditionalDelayingQueue(nil, time.Second, time.Second, 2*time.Second),
	}
}

type fakePodLister struct {
	get  *apiv1.Pod
	list []*apiv1.Pod
}

func (l *fakePodLister) Get(name string) (*apiv1.Pod, error) {
	if l.get == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "core/v1",
			Resource: "Pod",
		}, name)
	}
	return l.get, nil
}

func (l *fakePodLister) List(selector labels.Selector) (ret []*apiv1.Pod, err error) {
	return l.list, nil
}

func (l *fakePodLister) Pods(namespace string) v1.PodNamespaceLister {
	return l
}

type fakeLBLister struct {
	get  *lbcfapi.LoadBalancer
	list []*lbcfapi.LoadBalancer
}

func (l *fakeLBLister) Get(name string) (*lbcfapi.LoadBalancer, error) {
	if l.get == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "LoadBalancer",
		}, name)
	}
	return l.get, nil
}

func (l *fakeLBLister) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancer, err error) {
	return l.list, nil
}

func (l *fakeLBLister) LoadBalancers(namespace string) lbcflister.LoadBalancerNamespaceLister {
	return l
}

type fakeDriverLister struct {
	get  *lbcfapi.LoadBalancerDriver
	list []*lbcfapi.LoadBalancerDriver
}

func (l *fakeDriverLister) Get(name string) (*lbcfapi.LoadBalancerDriver, error) {
	if l.get == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "LoadBalancerDriver",
		}, name)
	}
	return l.get, nil
}

func (l *fakeDriverLister) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancerDriver, err error) {
	return l.list, nil
}

func (l *fakeDriverLister) LoadBalancerDrivers(namespace string) lbcflister.LoadBalancerDriverNamespaceLister {
	return l
}

type fakeBackendGroupLister struct {
	get  *lbcfapi.BackendGroup
	list []*lbcfapi.BackendGroup
}

func (l *fakeBackendGroupLister) Get(name string) (*lbcfapi.BackendGroup, error) {
	if l.get == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "BackendGroup",
		}, name)
	}
	return l.get, nil
}

func (l *fakeBackendGroupLister) BackendGroups(namespace string) lbcflister.BackendGroupNamespaceLister {
	return l
}

func (l *fakeBackendGroupLister) List(selector labels.Selector) (ret []*lbcfapi.BackendGroup, err error) {
	return l.list, nil
}

type fakeBackendLister struct {
	get  *lbcfapi.BackendRecord
	list []*lbcfapi.BackendRecord
}

func (l *fakeBackendLister) Get(name string) (*lbcfapi.BackendRecord, error) {
	if l.get == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "BackendRecord",
		}, name)
	}
	return l.get, nil
}

func (l *fakeBackendLister) List(selector labels.Selector) (ret []*lbcfapi.BackendRecord, err error) {
	return l.list, nil
}

func (l *fakeBackendLister) BackendRecords(namespace string) lbcflister.BackendRecordNamespaceLister {
	return l
}

func newFakeBackendListerWithStore() *fakeBackendListerWithStore {
	return &fakeBackendListerWithStore{
		store: make(map[string]*lbcfapi.BackendRecord),
	}
}

type fakeBackendListerWithStore struct {
	// map: name -> BackendRecord
	store map[string]*lbcfapi.BackendRecord
}

func (l *fakeBackendListerWithStore) Get(name string) (*lbcfapi.BackendRecord, error) {
	backend, ok := l.store[name]
	if !ok {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "BackendRecord",
		}, name)
	}
	return backend, nil
}

func (l *fakeBackendListerWithStore) List(selector labels.Selector) (ret []*lbcfapi.BackendRecord, err error) {
	for _, backend := range l.store {
		ret = append(ret, backend)
	}
	return
}

func (l *fakeBackendListerWithStore) BackendRecords(namespace string) lbcflister.BackendRecordNamespaceLister {
	return l
}

type fakeNodeListerWithStore struct {
	// map: name -> Node
	store map[string]*apiv1.Node
}

func (l *fakeNodeListerWithStore) ListWithPredicate(predicate v1.NodeConditionPredicate) ([]*apiv1.Node, error) {
	nodes, err := l.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var filtered []*apiv1.Node
	for i := range nodes {
		if predicate(nodes[i]) {
			filtered = append(filtered, nodes[i])
		}
	}

	return filtered, nil
}

func (l *fakeNodeListerWithStore) Get(name string) (*apiv1.Node, error) {
	if l.store == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "core/v1",
			Resource: "Node",
		}, name)
	}
	node, ok := l.store[name]
	if !ok {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "core/v1",
			Resource: "Node",
		}, name)
	}
	return node, nil
}

func (l *fakeNodeListerWithStore) List(selector labels.Selector) (ret []*apiv1.Node, err error) {
	for _, node := range l.store {
		ret = append(ret, node)
	}
	return
}

type fakeSvcListerWithStore struct {
	// map: name -> Service
	store map[string]*apiv1.Service
}

func (l *fakeSvcListerWithStore) Get(name string) (*apiv1.Service, error) {
	if l.store == nil {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "core/v1",
			Resource: "Service",
		}, name)
	}
	svc, ok := l.store[name]
	if !ok {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "core/v1",
			Resource: "Service",
		}, name)
	}
	return svc, nil
}

func (l *fakeSvcListerWithStore) List(selector labels.Selector) (ret []*apiv1.Service, err error) {
	for _, node := range l.store {
		ret = append(ret, node)
	}
	return
}

func (l *fakeSvcListerWithStore) Services(namespace string) v1.ServiceNamespaceLister {
	return l
}

func (l *fakeSvcListerWithStore) GetPodServices(pod *apiv1.Pod) ([]*apiv1.Service, error) {
	allServices, err := l.Services(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var services []*apiv1.Service
	for i := range allServices {
		service := allServices[i]
		if service.Spec.Selector == nil {
			// services with nil selectors match nothing, not everything.
			continue
		}
		selector := labels.Set(service.Spec.Selector).AsSelectorPreValidated()
		if selector.Matches(labels.Set(pod.Labels)) {
			services = append(services, service)
		}
	}

	return services, nil
}

type fakeSuccInvoker struct{}

func (c *fakeSuccInvoker) CallValidateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error) {
	return &webhooks.ValidateLoadBalancerResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: true,
			Msg:  "fake succ",
		},
	}, nil
}

func (c *fakeSuccInvoker) CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	return &webhooks.CreateLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusSucc,
			Msg:    "fake succ",
		},
	}, nil
}

func (c *fakeSuccInvoker) CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	return &webhooks.EnsureLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusSucc,
			Msg:    "fake succ",
		},
	}, nil
}

func (c *fakeSuccInvoker) CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	return &webhooks.DeleteLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusSucc,
			Msg:    "fake succ",
		},
	}, nil
}

func (c *fakeSuccInvoker) CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	return &webhooks.ValidateBackendResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: true,
			Msg:  "fake succ",
		},
	}, nil
}

func (c *fakeSuccInvoker) CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	return &webhooks.GenerateBackendAddrResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusSucc,
			Msg:    "fake succ",
		},
		BackendAddr: "fake.backend.addr.com:1234",
	}, nil
}

func (c *fakeSuccInvoker) CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusSucc,
			Msg:    "fake succ",
		},
	}, nil
}

func (c *fakeSuccInvoker) CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusSucc,
			Msg:    "fake succ",
		},
	}, nil
}

type fakeFailInvoker struct{}

func (c *fakeFailInvoker) CallValidateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error) {
	return &webhooks.ValidateLoadBalancerResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: false,
			Msg:  "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	return &webhooks.CreateLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusFail,
			Msg:    "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	return &webhooks.EnsureLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusFail,
			Msg:    "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	return &webhooks.DeleteLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusFail,
			Msg:    "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	return &webhooks.ValidateBackendResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: false,
			Msg:  "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	return &webhooks.GenerateBackendAddrResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusFail,
			Msg:    "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusFail,
			Msg:    "fake fail",
		},
	}, nil
}

func (c *fakeFailInvoker) CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status: webhooks.StatusFail,
			Msg:    "fake fail",
		},
	}, nil
}

func drainingDriverLister() lbcflister.LoadBalancerDriverLister {
	return &fakeDriverLister{
		get: &lbcfapi.LoadBalancerDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-driver",
				Labels: map[string]string{
					lbcfapi.DriverDrainingLabel: "True",
				},
			},
		},
	}
}

func deletingDriverLister() lbcflister.LoadBalancerDriverLister {
	ts := metav1.Now()
	return &fakeDriverLister{
		get: &lbcfapi.LoadBalancerDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-driver",
				DeletionTimestamp: &ts,
			},
		},
	}
}

type fakeRunningInvoker struct{}

func (c *fakeRunningInvoker) CallValidateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error) {
	return &webhooks.ValidateLoadBalancerResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: true,
			Msg:  "fake succ",
		},
	}, nil
}

func (c *fakeRunningInvoker) CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	return &webhooks.CreateLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 webhooks.StatusRunning,
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeRunningInvoker) CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	return &webhooks.EnsureLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 webhooks.StatusRunning,
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeRunningInvoker) CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	return &webhooks.DeleteLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 webhooks.StatusRunning,
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeRunningInvoker) CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	return &webhooks.ValidateBackendResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: true,
			Msg:  "fake succ",
		},
	}, nil
}

func (c *fakeRunningInvoker) CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	return &webhooks.GenerateBackendAddrResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 webhooks.StatusRunning,
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeRunningInvoker) CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 webhooks.StatusRunning,
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeRunningInvoker) CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 webhooks.StatusRunning,
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

type fakeInvalidInvoker struct{}

func (c *fakeInvalidInvoker) CallValidateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error) {
	return &webhooks.ValidateLoadBalancerResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: false,
			Msg:  "fake succ",
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	return &webhooks.CreateLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 "invalid status",
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	return &webhooks.EnsureLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 "invalid status",
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	return &webhooks.DeleteLoadBalancerResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 "invalid status",
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	return &webhooks.ValidateBackendResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: false,
			Msg:  "fake succ",
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	return &webhooks.GenerateBackendAddrResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 "invalid status",
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 "invalid status",
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

func (c *fakeInvalidInvoker) CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	return &webhooks.BackendOperationResponse{
		ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
			Status:                 "invalid status",
			Msg:                    "fake running",
			MinRetryDelayInSeconds: 60,
		},
	}, nil
}

type fakeEventRecorder struct {
	store map[string]string
}

func (r *fakeEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	//gvk := object.GetObjectKind().GroupVersionKind()
	access, _ := meta.Accessor(object)
	name := access.GetName()
	if r.store == nil {
		r.store = make(map[string]string)
	}
	r.store[name] = reason
}

func (r *fakeEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

func (r *fakeEventRecorder) PastEventf(object runtime.Object, timestamp metav1.Time, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (r *fakeEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}
