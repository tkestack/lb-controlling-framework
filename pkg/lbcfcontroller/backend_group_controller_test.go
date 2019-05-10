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
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"
	"reflect"
	"testing"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/controller"
)

func TestBackendGroupCreateRecord(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", pod1.Labels, nil, nil)
	fakeClient := fake.NewSimpleClientset(group)
	ctrl := NewBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		})
	key, _ := controller.KeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %#v", result)
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if get.Status.Backends != 2 {
		t.Fatalf("expect status.backends = 2, get %v", get.Status.Backends)
	} else if get.Status.RegisteredBackends != 0 {
		t.Fatalf("expect status.RegisteredBackends = 0, get %v", get.Status.RegisteredBackends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(group.Namespace).List(metav1.ListOptions{})
	if len(records.Items) != 2 {
		t.Fatalf("expect 2 BackendReocrds, get %v, %#v", len(records.Items), records.Items)
	}
	for _, r := range records.Items {
		var expected *lbcfapi.BackendRecord
		switch r.Name {
		case util.MakeBackendName(lb.Name, group.Name, pod1.Name, lbcfapi.PortSelector{PortNumber: 80}):
			expected = util.ConstructBackendRecord(lb, group, pod1.Name)
		case util.MakeBackendName(lb.Name, group.Name, pod2.Name, lbcfapi.PortSelector{PortNumber: 80}):
			expected = util.ConstructBackendRecord(lb, group, pod2.Name)
		default:
			t.Fatalf("unknown BackendRecord %#v", r)
		}
		if !reflect.DeepEqual(*expected, r) {
			t.Errorf("expect BackendRecord %#v \n get %#v", *expected, r)
		}
	}
}

func TestBackendGroupCreateRecordByPodName(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	pod1 := newFakePod("", "pod-1", nil, true, false)
	pod2 := newFakePod("", "pod-2", nil, true, false)
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-1"})
	fakeClient := fake.NewSimpleClientset(group)
	ctrl := NewBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		})
	key, _ := controller.KeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %#v", result)
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if get.Status.Backends != 1 {
		t.Fatalf("expect status.backends = 1, get %v", get.Status.Backends)
	} else if get.Status.RegisteredBackends != 0 {
		t.Fatalf("expect status.RegisteredBackends = 0, get %v", get.Status.RegisteredBackends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(group.Namespace).List(metav1.ListOptions{})
	if len(records.Items) != 1 {
		t.Fatalf("expect 1 BackendReocrds, get %v, %#v", len(records.Items), records.Items)
	}
	expect := util.ConstructBackendRecord(lb, group, pod1.Name)
	if !reflect.DeepEqual(*expect, records.Items[0]) {
		t.Errorf("expect BackendRecord %#v \n get %#v", *expect, records.Items[0])
	}
}

func TestBackendGroupUpdateRecordCausedByGroupUpdate(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	oldGroup := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", pod1.Labels, nil, nil)
	curGroup := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", pod1.Labels, nil, nil)
	curGroup.Spec.Parameters = map[string]string{
		"p1": "v1",
	}
	oldBackend1 := util.ConstructBackendRecord(lb, oldGroup, pod1.Name)
	oldBackend2 := util.ConstructBackendRecord(lb, oldGroup, pod2.Name)
	fakeClient := fake.NewSimpleClientset(curGroup, oldBackend1, oldBackend2)
	ctrl := NewBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: curGroup,
		},
		&fakeBackendLister{
			list: []*lbcfapi.BackendRecord{
				oldBackend1,
				oldBackend2,
			},
		},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		})
	key, _ := controller.KeyFunc(curGroup)
	result := ctrl.syncBackendGroup(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %#v", result)
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(curGroup.Namespace).Get(curGroup.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if get.Status.Backends != 2 {
		t.Fatalf("expect status.backends = 2, get %v", get.Status.Backends)
	} else if get.Status.RegisteredBackends != 0 {
		t.Fatalf("expect status.RegisteredBackends = 0, get %v", get.Status.RegisteredBackends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(curGroup.Namespace).List(metav1.ListOptions{})
	if len(records.Items) != 2 {
		t.Fatalf("expect 2 BackendReocrds, get %v, %#v", len(records.Items), records.Items)
	}
	for _, r := range records.Items {
		var expected *lbcfapi.BackendRecord
		switch r.Name {
		case util.MakeBackendName(lb.Name, curGroup.Name, pod1.Name, lbcfapi.PortSelector{PortNumber: 80}):
			expected = util.ConstructBackendRecord(lb, curGroup, pod1.Name)
		case util.MakeBackendName(lb.Name, curGroup.Name, pod2.Name, lbcfapi.PortSelector{PortNumber: 80}):
			expected = util.ConstructBackendRecord(lb, curGroup, pod2.Name)
		default:
			t.Fatalf("unknown BackendRecord %#v", r)
		}
		if !reflect.DeepEqual(*expected, r) {
			t.Errorf("expect BackendRecord %#v \n get %#v", *expected, r)
		}
	}
}

func TestBackendGroupUpdateRecordCausedByLBUpdate(t *testing.T) {
	oldLB := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	curLB := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	curLB.Spec.Attributes["a2"] = "v2"

	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)

	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", curLB.Name, 80, "tcp", pod1.Labels, nil, nil)
	oldBackend1 := util.ConstructBackendRecord(oldLB, group, pod1.Name)
	oldBackend2 := util.ConstructBackendRecord(oldLB, group, pod2.Name)
	fakeClient := fake.NewSimpleClientset(group, oldBackend1, oldBackend2)

	ctrl := NewBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  curLB,
			list: []*lbcfapi.LoadBalancer{curLB},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{
			list: []*lbcfapi.BackendRecord{
				oldBackend1,
				oldBackend2,
			},
		},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		})
	key, _ := controller.KeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %#v", result)
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if get.Status.Backends != 2 {
		t.Fatalf("expect status.backends = 2, get %v", get.Status.Backends)
	} else if get.Status.RegisteredBackends != 0 {
		t.Fatalf("expect status.RegisteredBackends = 0, get %v", get.Status.RegisteredBackends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(group.Namespace).List(metav1.ListOptions{})
	if len(records.Items) != 2 {
		t.Fatalf("expect 2 BackendReocrds, get %v, %#v", len(records.Items), records.Items)
	}
	for _, r := range records.Items {
		var expected *lbcfapi.BackendRecord
		switch r.Name {
		case util.MakeBackendName(curLB.Name, group.Name, pod1.Name, lbcfapi.PortSelector{PortNumber: 80}):
			expected = util.ConstructBackendRecord(curLB, group, pod1.Name)
		case util.MakeBackendName(curLB.Name, group.Name, pod2.Name, lbcfapi.PortSelector{PortNumber: 80}):
			expected = util.ConstructBackendRecord(curLB, group, pod2.Name)
		default:
			t.Fatalf("unknown BackendRecord %#v", r)
		}
		if !reflect.DeepEqual(*expected, r) {
			t.Errorf("expect BackendRecord %#v \n get %#v", *expected, r)
		}
	}
}

func TestBackendGroupDeleteRecordCausedByPodStatusChange(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)

	oldPod := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	curPod := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, false, true)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)

	group := newFakeBackendGroupOfPods(curPod.Namespace, "group", lb.Name, 80, "tcp", curPod.Labels, nil, nil)
	existingBackend1 := util.ConstructBackendRecord(lb, group, oldPod.Name)
	existingBackend2 := util.ConstructBackendRecord(lb, group, pod2.Name)

	fakeClient := fake.NewSimpleClientset(group, existingBackend1, existingBackend2)
	ctrl := NewBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{
			list: []*lbcfapi.BackendRecord{existingBackend1, existingBackend2},
		},
		&fakePodLister{
			get:  curPod,
			list: []*v1.Pod{curPod, pod2},
		})
	key, _ := controller.KeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %#v", result)
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if get.Status.Backends != 1 {
		t.Fatalf("expect status.backends = 2, get %v", get.Status.Backends)
	} else if get.Status.RegisteredBackends != 0 {
		t.Fatalf("expect status.RegisteredBackends = 0, get %v", get.Status.RegisteredBackends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(group.Namespace).List(metav1.ListOptions{})
	// since our fake client doesn't respect finalizers, so the record is deleted instantly
	if len(records.Items) != 1 {
		t.Fatalf("expect 1 BackendReocrds, get %v, %#v", len(records.Items), records.Items)
	}
	if records.Items[0].Name != util.MakeBackendName(lb.Name, group.Name, pod2.Name, lbcfapi.PortSelector{PortNumber: 80}) {
		t.Fatalf("wrong BackendRecord, get %v", records.Items[0])
	}
}

func TestBackendGroupDeleteRecordCausedByLBDeleted(t *testing.T) {
	ts := metav1.Now()
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	lb.DeletionTimestamp = &ts
	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", pod1.Labels, nil, nil)
	existingBackend1 := util.ConstructBackendRecord(lb, group, pod1.Name)
	existingBackend2 := util.ConstructBackendRecord(lb, group, pod2.Name)
	fakeClient := fake.NewSimpleClientset(group, existingBackend1, existingBackend2)
	ctrl := NewBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{
			list: []*lbcfapi.BackendRecord{
				existingBackend1,
				existingBackend2,
			},
		},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		})
	key, _ := controller.KeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %#v", result.GetError())
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if get.Status.Backends != 0 {
		t.Fatalf("expect status.backends = 0, get %v", get.Status.Backends)
	} else if get.Status.RegisteredBackends != 0 {
		t.Fatalf("expect status.RegisteredBackends = 0, get %v", get.Status.RegisteredBackends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(group.Namespace).List(metav1.ListOptions{})
	if len(records.Items) != 0 {
		t.Fatalf("expect 0 BackendReocrds, get %v, %#v", len(records.Items), records.Items)
	}
}
