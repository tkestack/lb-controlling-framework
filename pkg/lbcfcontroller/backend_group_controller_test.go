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
	"reflect"
	"testing"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestBackendGroupCreateRecord(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)
	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	pod2.UID = "anotherUID"
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "TCP", pod1.Labels, nil, nil)
	fakeClient := fake.NewSimpleClientset(group)
	ctrl := newBackendGroupController(
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
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
		t.Fatalf("expect succ result, get %#v", result.GetFailReason())
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
		var expected []*lbcfapi.BackendRecord
		switch r.Name {
		case util.MakePodBackendName(lb.Name, group.Name, pod1.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}):
			expected = util.ConstructPodBackendRecord(lb, group, pod1)
		case util.MakePodBackendName(lb.Name, group.Name, pod2.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}):
			expected = util.ConstructPodBackendRecord(lb, group, pod2)
		default:
			t.Fatalf("unknown BackendRecord %#v", r)
		}
		if !reflect.DeepEqual(*expected[0], r) {
			t.Errorf("expect BackendRecord %#v \n get %#v", *expected[0], r)
		}
	}
}

func TestBackendGroupCreateRecordByPodName(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)
	pod1 := newFakePod("", "pod-1", nil, true, false)
	pod2 := newFakePod("", "pod-2", nil, true, false)
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", nil, nil, []string{"pod-1"})
	fakeClient := fake.NewSimpleClientset(group)
	ctrl := newBackendGroupController(
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
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
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
	expect := util.ConstructPodBackendRecord(lb, group, pod1)
	if !reflect.DeepEqual(*expect[0], records.Items[0]) {
		t.Errorf("expect BackendRecord %#v \n get %#v", *expect[0], records.Items[0])
	}
}

func TestBackendGroupCreateRecordByService(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)
	svc := newFakeService("", "test-svc", v1.ServiceTypeNodePort)
	group := newFakeBackendGroupOfService("", "test-group", lb.Name, 80, "TCP", svc.Name)
	fakeClient := fake.NewSimpleClientset(group)
	ctrl := newBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{},
		&fakePodLister{},
		&fakeSvcListerWithStore{
			store: map[string]*v1.Service{
				svc.Name: svc,
			},
		},
		&fakeNodeListerWithStore{
			store: map[string]*v1.Node{
				"node1": newFakeNode("", "node1"),
				"node2": newFakeNode("", "node2"),
			},
		},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
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
}

func TestBackendGroupCreateRecordByServiceNotAvailable(t *testing.T) {
	type testCase struct {
		name string

		group *lbcfapi.BackendGroup
		svc   *v1.Service
		nodes []*v1.Node

		expectTotalBackends int
		expectRecords       int
	}

	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)

	cases := []testCase{
		{
			name:                "svc-not-found",
			group:               newFakeBackendGroupOfService("", "test-group", lb.Name, 80, "TCP", "not-exist-svc"),
			nodes:               []*v1.Node{newFakeNode("", "node1"), newFakeNode("", "node2")},
			expectTotalBackends: 0,
			expectRecords:       0,
		},
		{
			name:                "svc-not-NodePort",
			group:               newFakeBackendGroupOfService("", "test-group", lb.Name, 80, "TCP", "not-exist-svc"),
			svc:                 newFakeService("", "test-svc", v1.ServiceTypeClusterIP),
			nodes:               []*v1.Node{newFakeNode("", "node1"), newFakeNode("", "node2")},
			expectTotalBackends: 0,
			expectRecords:       0,
		},
	}
	for _, c := range cases {
		fakeClient := fake.NewSimpleClientset(c.group)
		nodeStore := make(map[string]*v1.Node)
		for _, node := range c.nodes {
			nodeStore[node.Name] = node
		}
		svcSotre := make(map[string]*v1.Service)
		if c.svc != nil {
			svcSotre[c.svc.Name] = c.svc
		}
		ctrl := newBackendGroupController(
			fakeClient,
			&fakeLBLister{
				get:  lb,
				list: []*lbcfapi.LoadBalancer{lb},
			},
			&fakeBackendGroupLister{
				get: c.group,
			},
			&fakeBackendLister{},
			&fakePodLister{},
			&fakeSvcListerWithStore{
				store: svcSotre,
			},
			&fakeNodeListerWithStore{
				store: nodeStore,
			},
			false,
		)
		key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(c.group)
		result := ctrl.syncBackendGroup(key)
		if !result.IsFinished() {
			t.Fatalf("case %s: expect succ result, get %#v", c.name, result)
		}
		if get, _ := fakeClient.LbcfV1beta1().BackendGroups(c.group.Namespace).Get(c.group.Name, metav1.GetOptions{}); get == nil {
			t.Fatalf("case %s: miss BackendGroup", c.name)
		} else if get.Status.Backends != int32(c.expectTotalBackends) {
			t.Fatalf("case %s: expect status.backends = 0, get %v", c.name, get.Status.Backends)
		}

		records, _ := fakeClient.LbcfV1beta1().BackendRecords(c.group.Namespace).List(metav1.ListOptions{})
		if len(records.Items) != c.expectRecords {
			t.Fatalf("case %s: expect %d BackendReocrds, get %v", c.name, c.expectRecords, len(records.Items))
		}
	}
}

func TestBackendGroupCreateRecordByStatic(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)
	staticAddrs := []string{
		"addr1",
		"addr2",
	}
	group := newFakeBackendGroupOfStatic("", "test-group", lb.Name, staticAddrs...)
	fakeClient := fake.NewSimpleClientset(group)
	ctrl := newBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{},
		&fakePodLister{},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
		t.Fatalf("expect succ result, get %#v", result)
	}

	if get, _ := fakeClient.LbcfV1beta1().BackendGroups(group.Namespace).Get(group.Name, metav1.GetOptions{}); get == nil {
		t.Fatalf("miss BackendGroup")
	} else if int(get.Status.Backends) != len(staticAddrs) {
		t.Fatalf("expect status.backends = %d, get %v", len(staticAddrs), get.Status.Backends)
	}

	records, _ := fakeClient.LbcfV1beta1().BackendRecords(group.Namespace).List(metav1.ListOptions{})
	if len(records.Items) != len(staticAddrs) {
		t.Fatalf("expect %d BackendReocrds, get %v", len(staticAddrs), len(records.Items))
	}
}

func TestBackendGroupUpdateRecordCausedByGroupUpdate(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)
	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	pod2.UID = "anotherUID"
	oldGroup := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "TCP", pod1.Labels, nil, nil)
	curGroup := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "TCP", pod1.Labels, nil, nil)
	curGroup.Spec.Parameters = map[string]string{
		"p1": "v1",
	}
	oldBackend1 := util.ConstructPodBackendRecord(lb, oldGroup, pod1)
	oldBackend2 := util.ConstructPodBackendRecord(lb, oldGroup, pod2)
	fakeClient := fake.NewSimpleClientset(curGroup, oldBackend1[0], oldBackend2[0])
	ctrl := newBackendGroupController(
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
				oldBackend1[0],
				oldBackend2[0],
			},
		},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(curGroup)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
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
		var expected []*lbcfapi.BackendRecord
		switch r.Name {
		case util.MakePodBackendName(lb.Name, curGroup.Name, pod1.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}):
			expected = util.ConstructPodBackendRecord(lb, curGroup, pod1)
		case util.MakePodBackendName(lb.Name, curGroup.Name, pod2.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}):
			expected = util.ConstructPodBackendRecord(lb, curGroup, pod2)
		default:
			t.Fatalf("unknown BackendRecord %#v", r)
		}
		if !reflect.DeepEqual(*expected[0], r) {
			t.Errorf("expect BackendRecord %#v \n get %#v", *expected[0], r)
		}
	}
}

func TestBackendGroupUpdateRecordCausedByLBUpdate(t *testing.T) {
	oldLB := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(oldLB)
	curLB := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	curLB.Spec.Attributes["a2"] = "v2"
	fakeLBEnsured(curLB)

	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	pod2.UID = "anotherUID"
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", curLB.Name, 80, "TCP", pod1.Labels, nil, nil)
	oldBackend1 := util.ConstructPodBackendRecord(oldLB, group, pod1)
	oldBackend2 := util.ConstructPodBackendRecord(oldLB, group, pod2)
	fakeClient := fake.NewSimpleClientset(group, oldBackend1[0], oldBackend2[0])

	ctrl := newBackendGroupController(
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
				oldBackend1[0],
				oldBackend2[0],
			},
		},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
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
		var expected []*lbcfapi.BackendRecord
		switch r.Name {
		case util.MakePodBackendName(curLB.Name, group.Name, pod1.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}):
			expected = util.ConstructPodBackendRecord(curLB, group, pod1)
		case util.MakePodBackendName(curLB.Name, group.Name, pod2.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}):
			expected = util.ConstructPodBackendRecord(curLB, group, pod2)
		default:
			t.Fatalf("unknown BackendRecord %#v", r)
		}
		if !reflect.DeepEqual(*expected[0], r) {
			t.Errorf("expect BackendRecord %#v \n get %#v", *expected[0], r)
		}
	}
}

func TestBackendGroupDeleteRecordCausedByPodStatusChange(t *testing.T) {
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	fakeLBEnsured(lb)
	oldPod := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	curPod := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, false, true)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	pod2.UID = "anotherUID"

	group := newFakeBackendGroupOfPods(curPod.Namespace, "group", lb.Name, 80, "TCP", curPod.Labels, nil, nil)
	existingBackend1 := util.ConstructPodBackendRecord(lb, group, oldPod)
	existingBackend2 := util.ConstructPodBackendRecord(lb, group, pod2)

	fakeClient := fake.NewSimpleClientset(group, existingBackend1[0], existingBackend2[0])
	ctrl := newBackendGroupController(
		fakeClient,
		&fakeLBLister{
			get:  lb,
			list: []*lbcfapi.LoadBalancer{lb},
		},
		&fakeBackendGroupLister{
			get: group,
		},
		&fakeBackendLister{
			list: []*lbcfapi.BackendRecord{existingBackend1[0], existingBackend2[0]},
		},
		&fakePodLister{
			get:  curPod,
			list: []*v1.Pod{curPod, pod2},
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
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
	if records.Items[0].Name != util.MakePodBackendName(lb.Name, group.Name, pod2.UID, lbcfapi.PortSelector{Port: 80, Protocol: "TCP"}) {
		t.Fatalf("wrong BackendRecord, get %v", records.Items[0])
	}
}

func TestBackendGroupDeleteRecordCausedByLBDeleted(t *testing.T) {
	ts := metav1.Now()
	lb := newFakeLoadBalancer("", "lb", map[string]string{"a1": "v1"}, nil)
	lb.DeletionTimestamp = &ts
	pod1 := newFakePod("", "pod-1", map[string]string{"k1": "v1"}, true, false)
	pod2 := newFakePod("", "pod-2", map[string]string{"k1": "v1"}, true, false)
	pod2.UID = "anotherUID"
	group := newFakeBackendGroupOfPods(pod1.Namespace, "group", lb.Name, 80, "tcp", pod1.Labels, nil, nil)
	existingBackend1 := util.ConstructPodBackendRecord(lb, group, pod1)
	existingBackend2 := util.ConstructPodBackendRecord(lb, group, pod2)
	fakeClient := fake.NewSimpleClientset(group, existingBackend1[0], existingBackend2[0])
	ctrl := newBackendGroupController(
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
				existingBackend1[0],
				existingBackend2[0],
			},
		},
		&fakePodLister{
			get:  pod1,
			list: []*v1.Pod{pod1, pod2},
		},
		&fakeSvcListerWithStore{},
		&fakeNodeListerWithStore{},
		false,
	)
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(group)
	result := ctrl.syncBackendGroup(key)
	if !result.IsFinished() {
		t.Fatalf("expect succ result, get %#v", result.GetFailReason())
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

func fakeLBEnsured(lb *lbcfapi.LoadBalancer) {
	ts := metav1.Now()
	util.AddLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
		Type:               lbcfapi.LBCreated,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	})
	util.AddLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
		Type:               lbcfapi.LBAttributesSynced,
		Status:             lbcfapi.ConditionTrue,
		LastTransitionTime: ts,
	})
}
