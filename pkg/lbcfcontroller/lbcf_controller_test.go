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
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/controller"
	"strings"
	"testing"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	lbcflister "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

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
		&fakePodLister{})
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

	oldPod2 := newFakePod("", "pod-0", podLabel, true, false)
	curPod2 := newFakePod("", "pod-0", podLabel, false, false)

	oldPod3 := newFakePod("", "pod-0", podLabel, true, false)
	curPod3 := newFakePod("", "pod-0", podLabel, false, true)

	bg1 := newFakeBackendGroupOfPods("", "bg-0", "", 80, "tcp", podLabel, nil, nil)

	bgCtrl := newBackendGroupController(
		fake.NewSimpleClientset(),
		&fakeLBLister{},
		&fakeBackendGroupLister{
			list: []*lbcfapi.BackendGroup{bg1},
		},
		&fakeBackendLister{},
		&fakePodLister{})
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

	oldPod2 := newFakePod("", "pod-2", podLabel1, true, false)
	curPod2 := newFakePod("", "pod-2", nil, true, false)

	oldPod3 := newFakePod("", "pod-3", podLabel1, true, false)
	curPod3 := newFakePod("", "pod-3", podLabel2, true, false)

	oldPod4 := newFakePod("", "pod-3", podLabel1, true, false)
	curPod4 := newFakePod("", "pod-3", podLabel1Plus, true, false)

	bg1 := newFakeBackendGroupOfPods("", "bg-1", "", 80, "tcp", podLabel1, nil, nil)
	bg2 := newFakeBackendGroupOfPods("", "bg-2", "", 80, "tcp", podLabel2, nil, nil)

	bgCtrl := newBackendGroupController(
		fake.NewSimpleClientset(),
		&fakeLBLister{},
		&fakeBackendGroupLister{
			list: []*lbcfapi.BackendGroup{bg1, bg2},
		},
		&fakeBackendLister{},
		&fakePodLister{})
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
		&fakePodLister{})
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
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{}, &fakeBackendLister{}, &fakePodLister{})
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
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{}, &fakeBackendLister{}, &fakePodLister{})
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
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{}, &fakeBackendLister{}, &fakePodLister{})
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

func TestLBCFControllerAddLoadBalancer(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods(lb.Namespace, "bg", lb.Name, 80, "tcp", nil, nil, nil)
	bg2 := newFakeBackendGroupOfPods(lb.Namespace, "bg", "another-lb", 80, "tcp", nil, nil, nil)

	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{})
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
	attr1 := map[string]string{
		"a1": "v1",
	}
	attr2 := map[string]string{
		"a2": "v2",
	}
	oldLB1 := newFakeLoadBalancer("", "lb-1", attr1, nil)
	curLB1 := newFakeLoadBalancer("", "lb-1", attr2, nil)
	curLB1.Generation = 2
	bg := newFakeBackendGroupOfPods("", "bg", "lb-1", 80, "tcp", nil, nil, nil)

	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg},
	}, &fakeBackendLister{}, &fakePodLister{})
	c := newFakeLBCFController(nil, lbCtrl, nil, bgCtrl)

	c.updateLoadBalancer(oldLB1, curLB1)
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
	} else if expectedKey, _ := controller.KeyFunc(curLB1); expectedKey != key {
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

//func TestLBCFControllerUpdateLoadBalancer_StatusChanged(t *testing.T) {
//	oldLB1 := newFakeLoadBalancer("", "lb-1", nil, nil)
//	curLB1 := newFakeLoadBalancer("", "lb-1", nil, nil)
//	ts := metav1.Now()
//	curLB1.DeletionTimestamp = &ts
//	curLB1.Status = lbcfapi.LoadBalancerStatus{
//		Conditions: []lbcfapi.LoadBalancerCondition{
//			{
//				Type:   lbcfapi.LBReadyToDelete,
//				Status: lbcfapi.ConditionTrue,
//			},
//		},
//	}
//
//	bg := newFakeBackendGroupOfPods("", "bg", "lb-1", 80, "tcp", nil, nil, nil)
//	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeSuccInvoker{})
//	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
//		list: []*lbcfapi.BackendGroup{bg},
//	}, &fakeBackendLister{}, &fakePodLister{})
//	c := newFakeLBCFController(nil, lbCtrl, nil, bgCtrl)
//
//	c.updateLoadBalancer(oldLB1, curLB1)
//	if c.loadBalancerQueue.Len() != 1 {
//		t.Fatalf("queue length should be 1, get %d", c.loadBalancerQueue.Len())
//	} else if c.backendGroupQueue.Len() != 1 {
//		t.Fatalf("queue length should be 1, get %d", c.backendGroupQueue.Len())
//	}
//}

func TestLBCFControllerDeleteLoadBalancer(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	bg := newFakeBackendGroupOfPods(lb.Namespace, "bg", lb.Name, 80, "tcp", nil, nil, nil)
	bg2 := newFakeBackendGroupOfPods(lb.Namespace, "bg", "another-lb", 80, "tcp", nil, nil, nil)
	tomestoneKey, _ := controller.KeyFunc(bg)
	tombstone := cache.DeletedFinalStateUnknown{Key: tomestoneKey, Obj: lb}

	lbCtrl := newLoadBalancerController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeDriverLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	bgCtrl := newBackendGroupController(fake.NewSimpleClientset(), &fakeLBLister{}, &fakeBackendGroupLister{
		list: []*lbcfapi.BackendGroup{bg, bg2},
	}, &fakeBackendLister{}, &fakePodLister{})
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
	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
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
		name        string
		old         *lbcfapi.BackendRecord
		cur         *lbcfapi.BackendRecord
		ctrl        *Controller
		expectQueue int
	}
	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	cases := []testCase{
		{
			name:        "same-resource-version-0",
			old:         newFakeBackendRecord("", "record"),
			cur:         newFakeBackendRecord("", "record"),
			ctrl:        newFakeLBCFController(nil, nil, backendCtrl, nil),
			expectQueue: 0,
		},
		{
			name: "same-resource-version-set-backendaddr-1",
			old:  newFakeBackendRecord("", "record"),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				return record
			}(),
			ctrl:        newFakeLBCFController(nil, nil, backendCtrl, nil),
			expectQueue: 1,
		},
		{
			name: "different-resource-version-1",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.ResourceVersion = "2"
				record.Status.BackendAddr = "anotheraddr.com"
				return record
			}(),
			ctrl:        newFakeLBCFController(nil, nil, backendCtrl, nil),
			expectQueue: 1,
		},
		{
			name: "same-resource-version-update-status-0",
			old: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				return record
			}(),
			cur: func() *lbcfapi.BackendRecord {
				record := newFakeBackendRecord("", "record")
				record.Status.BackendAddr = "fakeaddr.com"
				record.Status.Conditions = []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendRegistered,
						Status: lbcfapi.ConditionFalse,
					},
				}
				return record
			}(),
			ctrl:        newFakeLBCFController(nil, nil, backendCtrl, nil),
			expectQueue: 0,
		},
	}
	for _, c := range cases {
		c.ctrl.updateBackendRecord(c.old, c.cur)
		if c.ctrl.backendQueue.Len() != c.expectQueue {
			t.Errorf("expect len %d, get %d", c.expectQueue, c.ctrl.backendQueue.Len())
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
	}
}

func TestLBCFControllerUpdateBackendRecordRegisterStatusChanged(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	group := newFakeBackendGroupOfPods(lb.Namespace, "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	oldRecord := util.ConstructBackendRecord(lb, group, newFakePod("", "pod-0", nil, true, false))
	curRecord := util.ConstructBackendRecord(lb, group, newFakePod("", "pod-0", nil, true, false))
	curRecord.Status.BackendAddr = "fake.addr.com:80"
	curRecord.Status.Conditions = []lbcfapi.BackendRecordCondition{
		{
			Type:   lbcfapi.BackendRegistered,
			Status: lbcfapi.ConditionTrue,
		},
	}

	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
	c := newFakeLBCFController(nil, nil, backendCtrl, nil)
	c.updateBackendRecord(oldRecord, curRecord)

	if c.backendQueue.Len() != 1 {
		t.Fatalf("expect empty backend queue, get %d", c.backendQueue.Len())
	}

	if c.backendGroupQueue.Len() != 1 {
		t.Fatalf("expect backendgroup queue 1, get %d", c.backendGroupQueue.Len())
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
}

func TestLBCFControllerDeleteBackendRecord(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", nil, nil)
	group := newFakeBackendGroupOfPods(lb.Namespace, "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-0"})
	record := util.ConstructBackendRecord(lb, group, newFakePod("", "pod-0", nil, true, false))
	backendCtrl := newBackendController(fake.NewSimpleClientset(), &fakeBackendLister{}, &fakeDriverLister{}, &fakePodLister{}, &fakeEventRecorder{}, &fakeSuccInvoker{})
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
	q := util.NewIntervalRateLimitingQueue(
		workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 10*time.Minute),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "name", time.Second)
	obj := newFakeDriver("", "driver")
	ctrl.enqueue(obj, q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.SuccResult()
	})
	select {
	case <-time.NewTimer(2 * time.Second).C:
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() {
			q.Get()
			close(ch)
		}()
		return ch
	}():
		t.Fatalf("expect get nothing")
	}
}

func TestLBCFControllerProcessNextItemError(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewIntervalRateLimitingQueue(
		workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 10*time.Minute),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "name", time.Second)

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
	q := util.NewIntervalRateLimitingQueue(
		workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 10*time.Minute),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "name", time.Second)

	obj := newFakeDriver("", "driver")
	key, _ := controller.KeyFunc(obj)

	ctrl.enqueue(obj, q)
	ctrl.processNextItem(q, func(key string) *util.SyncResult {
		return util.FailResult(500 * time.Millisecond)
	})
	if get, done := q.Get(); done {
		t.Fatalf("failed to get queue elements")
	} else if get != key {
		t.Fatalf("expect key %q, get %q", key, get)
	}
}

func TestLBCFControllerProcessNextItemRunning(t *testing.T) {
	ctrl := &Controller{}
	q := util.NewIntervalRateLimitingQueue(
		workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 10*time.Minute),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "name", time.Second)
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
	q := util.NewIntervalRateLimitingQueue(
		workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 10*time.Minute),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		), "name", time.Second)
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
					Protocol:   &protocol,
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

func newFakeLBCFController(driverCtrl *driverController, lbCtrl *loadBalancerController, backendCtrl *backendController, bgCtrl *backendGroupController) *Controller {
	return &Controller{
		driverCtrl:       driverCtrl,
		lbCtrl:           lbCtrl,
		backendCtrl:      backendCtrl,
		backendGroupCtrl: bgCtrl,

		driverQueue:       util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "driver-queue", 10*time.Second),
		loadBalancerQueue: util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "lb-queue", 10*time.Second),
		backendGroupQueue: util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "backendgroup-queue", 10*time.Second),
		backendQueue:      util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "backend-queue", 10*time.Second),
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
