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

package util

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
	"testing"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodAvailable(t *testing.T) {
	deletionTimestamp := &metav1.Time{
		Time: time.Now(),
	}
	shouldBind := []v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: nil,
			},
			Status: v1.PodStatus{
				PodIP: "1.1.1.1",
				Conditions: []v1.PodCondition{
					{
						Type:   v1.PodReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
	}
	shouldNotBind := []v1.Pod{
		// deletionTimestamp is set
		{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: deletionTimestamp,
			},
			Status: v1.PodStatus{
				PodIP: "1.1.1.1",
				Conditions: []v1.PodCondition{
					{
						Type:   v1.PodReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
		// podIP is empty
		{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: nil,
			},
			Status: v1.PodStatus{
				PodIP: "",
				Conditions: []v1.PodCondition{
					{
						Type:   v1.PodReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
		// condition is not ready
		{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: nil,
			},
			Status: v1.PodStatus{
				PodIP: "1.1.1.1",
				Conditions: []v1.PodCondition{
					{
						Type:   v1.PodReady,
						Status: v1.ConditionFalse,
					},
				},
			},
		},
		// empty condition
		{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: nil,
			},
			Status: v1.PodStatus{
				PodIP: "1.1.1.1",
			},
		},
	}
	for _, pod := range shouldBind {
		if !PodAvailable(&pod) {
			t.Fatalf("pod should bind, but return false, pod: %+v", pod)
		}
	}
	for _, pod := range shouldNotBind {
		if PodAvailable(&pod) {
			t.Fatalf("pod should not bind, but return true, pod: %+v", pod)
		}
	}
}

func TestLBCreated(t *testing.T) {
	created := []*lbcfapi.LoadBalancer{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "created",
			},
			Spec: lbcfapi.LoadBalancerSpec{
				LBDriver: "my-driver",
				LBSpec: map[string]string{
					"id": "lbid-12234",
				},
			},
			Status: lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
				},
			},
		},
	}

	notCreated := []*lbcfapi.LoadBalancer{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-created",
			},
			Spec: lbcfapi.LoadBalancerSpec{
				LBDriver: "my-driver",
				LBSpec: map[string]string{
					"id": "lbid-12234",
				},
			},
			Status: lbcfapi.LoadBalancerStatus{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-created",
			},
			Spec: lbcfapi.LoadBalancerSpec{
				LBDriver: "my-driver",
				LBSpec: map[string]string{
					"id": "lbid-12234",
				},
			},
			Status: lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionFalse,
					},
				},
			},
		},
	}
	for i, lb := range created {
		if !LBCreated(lb) {
			t.Fatalf("expect created, index: %d", i)
		}
	}
	for i, lb := range notCreated {
		if LBCreated(lb) {
			t.Fatalf("expect not-created, index: %d", i)
		}
	}
}

func TestAddLBCondition(t *testing.T) {
	type lbConditionTest struct {
		name      string
		status    *lbcfapi.LoadBalancerStatus
		condition lbcfapi.LoadBalancerCondition
		expect    *lbcfapi.LoadBalancerStatus
	}

	testCases := []lbConditionTest{
		{
			name:   "add-condition-to-empty",
			status: &lbcfapi.LoadBalancerStatus{},
			condition: lbcfapi.LoadBalancerCondition{
				Type:    lbcfapi.LBEnsured,
				Status:  lbcfapi.ConditionTrue,
				Reason:  lbcfapi.ReasonOperationInProgress.String(),
				Message: "ensured",
			},
			expect: &lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:    lbcfapi.LBEnsured,
						Status:  lbcfapi.ConditionTrue,
						Reason:  lbcfapi.ReasonOperationInProgress.String(),
						Message: "ensured",
					},
				},
			},
		},
		{
			name: "add-condition",
			status: &lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
				},
			},
			condition: lbcfapi.LoadBalancerCondition{
				Type:    lbcfapi.LBEnsured,
				Status:  lbcfapi.ConditionTrue,
				Reason:  lbcfapi.ReasonOperationInProgress.String(),
				Message: "ensured",
			},
			expect: &lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:    lbcfapi.LBEnsured,
						Status:  lbcfapi.ConditionTrue,
						Reason:  lbcfapi.ReasonOperationInProgress.String(),
						Message: "ensured",
					},
					{
						Type:   lbcfapi.LBCreated,
						Status: lbcfapi.ConditionTrue,
					},
				},
			},
		},
		{
			name: "overwrite-condition",
			status: &lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:    lbcfapi.LBEnsured,
						Status:  lbcfapi.ConditionFalse,
						Reason:  lbcfapi.ReasonOperationInProgress.String(),
						Message: "should-be-overwrite",
					},
				},
			},
			condition: lbcfapi.LoadBalancerCondition{
				Type:    lbcfapi.LBEnsured,
				Status:  lbcfapi.ConditionTrue,
				Message: "overwrite",
			},
			expect: &lbcfapi.LoadBalancerStatus{
				Conditions: []lbcfapi.LoadBalancerCondition{
					{
						Type:    lbcfapi.LBEnsured,
						Status:  lbcfapi.ConditionTrue,
						Message: "overwrite",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		AddLBCondition(tc.status, tc.condition)
		if len(tc.status.Conditions) != len(tc.expect.Conditions) {
			t.Fatalf("case: %s. wrong length, expect: %d, get: %d", tc.name, len(tc.expect.Conditions), len(tc.status.Conditions))
		}
		for _, c := range tc.expect.Conditions {
			get := GetLBCondition(tc.status, c.Type)
			if get == nil {
				t.Fatalf("case: %s. not found", tc.name)
				continue
			}
			if *get != c {
				t.Fatalf("case: %s, condition not equal, expect: %+v, get: %+v", tc.name, c, *get)
			}
		}
	}
}

func TestAddBackendCondition(t *testing.T) {
	type backendConditionTest struct {
		name      string
		status    *lbcfapi.BackendRecordStatus
		condition lbcfapi.BackendRecordCondition
		expect    *lbcfapi.BackendRecordStatus
	}

	testCases := []backendConditionTest{
		{
			name:   "add-condition-to-empty",
			status: &lbcfapi.BackendRecordStatus{},
			condition: lbcfapi.BackendRecordCondition{
				Type:    lbcfapi.BackendAddrGenerated,
				Status:  lbcfapi.ConditionTrue,
				Reason:  lbcfapi.ReasonOperationInProgress.String(),
				Message: "ensured",
			},
			expect: &lbcfapi.BackendRecordStatus{
				Conditions: []lbcfapi.BackendRecordCondition{
					{
						Type:    lbcfapi.BackendAddrGenerated,
						Status:  lbcfapi.ConditionTrue,
						Reason:  lbcfapi.ReasonOperationInProgress.String(),
						Message: "ensured",
					},
				},
			},
		},
		{
			name: "add-condition",
			status: &lbcfapi.BackendRecordStatus{
				Conditions: []lbcfapi.BackendRecordCondition{
					{
						Type:   lbcfapi.BackendAddrGenerated,
						Status: lbcfapi.ConditionTrue,
					},
				},
			},
			condition: lbcfapi.BackendRecordCondition{
				Type:    lbcfapi.BackendRegistered,
				Status:  lbcfapi.ConditionTrue,
				Reason:  lbcfapi.ReasonOperationInProgress.String(),
				Message: "message",
			},
			expect: &lbcfapi.BackendRecordStatus{
				Conditions: []lbcfapi.BackendRecordCondition{
					{
						Type:    lbcfapi.BackendRegistered,
						Status:  lbcfapi.ConditionTrue,
						Reason:  lbcfapi.ReasonOperationInProgress.String(),
						Message: "message",
					},
					{
						Type:   lbcfapi.BackendAddrGenerated,
						Status: lbcfapi.ConditionTrue,
					},
				},
			},
		},
		{
			name: "overwrite-condition",
			status: &lbcfapi.BackendRecordStatus{
				Conditions: []lbcfapi.BackendRecordCondition{
					{
						Type:    lbcfapi.BackendRegistered,
						Status:  lbcfapi.ConditionFalse,
						Reason:  lbcfapi.ReasonOperationInProgress.String(),
						Message: "should-be-overwrite",
					},
				},
			},
			condition: lbcfapi.BackendRecordCondition{
				Type:    lbcfapi.BackendRegistered,
				Status:  lbcfapi.ConditionTrue,
				Message: "overwrite",
			},
			expect: &lbcfapi.BackendRecordStatus{
				Conditions: []lbcfapi.BackendRecordCondition{
					{
						Type:    lbcfapi.BackendRegistered,
						Status:  lbcfapi.ConditionTrue,
						Message: "overwrite",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		AddBackendCondition(tc.status, tc.condition)
		if len(tc.status.Conditions) != len(tc.expect.Conditions) {
			t.Fatalf("case: %s. wrong length, expect: %d, get: %d", tc.name, len(tc.expect.Conditions), len(tc.status.Conditions))
		}
		for _, c := range tc.expect.Conditions {
			found := false
			for i := range tc.status.Conditions {
				if tc.status.Conditions[i].Type == c.Type {
					found = true
					if tc.status.Conditions[i] != c {
						t.Fatalf("case: %s, condition not equal, expect: %+v, get: %+v", tc.name, c, tc.status.Conditions[i])
					}
				}
			}
			if !found {
				t.Fatalf("case: %s. not found", tc.name)
				continue
			}
		}
	}
}

func TestGetBackendType(t *testing.T) {
	type testCase struct {
		name         string
		backendGroup *lbcfapi.BackendGroup
		backendType  BackendType
	}

	cases := []testCase{
		{
			name: "service-backend",
			backendGroup: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Service: &lbcfapi.ServiceBackend{
						Name: "my-service",
						Port: lbcfapi.PortSelector{
							PortNumber: 8080,
						},
						NodeSelector: map[string]string{
							"key1": "value1",
						},
					},
				},
			},
			backendType: TypeService,
		},
		{
			name: "pod-backend",
			backendGroup: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 8080,
						},
						ByName: []string{
							"pod-1",
						},
					},
				},
			},
			backendType: TypePod,
		},
		{
			name: "static-backend",
			backendGroup: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Static: []string{
						"pod-1",
						"pod-2",
					},
				},
			},
			backendType: TypeStatic,
		},
		{
			name: "unknown-backend",
			backendGroup: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Static: []string{},
				},
			},
			backendType: TypeUnknown,
		},
	}
	for _, c := range cases {
		if get := GetBackendType(c.backendGroup); get != c.backendType {
			t.Fatalf("case %s: expect type %s, get %s", c.name, c.backendType, get)
		}
	}
}

func TestGetDriverNamespace(t *testing.T) {
	type testCase struct {
		name            string
		driverName      string
		namespace       string
		expectNamespace string
	}

	cases := []testCase{
		{
			name:            "test-case-1",
			driverName:      lbcfapi.SystemDriverPrefix + "aaa",
			namespace:       "kube-system",
			expectNamespace: "kube-system",
		},
		{
			name:            "test-case-2",
			driverName:      "my-driver",
			namespace:       "test",
			expectNamespace: "test",
		},
	}
	for _, c := range cases {
		if get := GetDriverNamespace(c.driverName, c.namespace); get != c.expectNamespace {
			t.Fatalf("case %s: expect %s, get %s", c.name, c.expectNamespace, get)
		}
	}
}

func TestIsDriverDraining(t *testing.T) {
	type testCases struct {
		name   string
		driver *lbcfapi.LoadBalancerDriver
		expect bool
	}

	cases := []testCases{
		{
			name: "draining",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						lbcfapi.DriverDrainingLabel: "true",
					},
				},
			},
			expect: true,
		},
		{
			name: "draining2",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						lbcfapi.DriverDrainingLabel: "True",
					},
				},
			},
			expect: true,
		},
		{
			name: "not-draining",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			expect: false,
		},
		{
			name: "not-draining2",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						lbcfapi.DriverDrainingLabel: "False",
					},
				},
			},
			expect: false,
		},
	}

	for _, c := range cases {
		if get := IsDriverDraining(c.driver.Labels); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestCalculateRetryInterval(t *testing.T) {
	type testCase struct {
		name      string
		userValue int32
		expected  time.Duration
	}

	cases := []testCase{
		{
			name:      "normal",
			userValue: 10,
			expected:  10 * time.Second,
		},
		{
			name:      "user-not-specified",
			userValue: 0,
			expected:  DefaultRetryInterval,
		},
		{
			name:      "invalid",
			userValue: -1,
			expected:  DefaultRetryInterval,
		},
	}

	for _, c := range cases {
		if get := CalculateRetryInterval(c.userValue); get != c.expected {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expected, get)
		}
	}
}

func TestHasFinalizer(t *testing.T) {
	type testCase struct {
		name    string
		all     []string
		lookfor string
		expect  bool
	}

	cases := []testCase{
		{
			name: "true",
			all: []string{
				"a", "b", "c",
			},
			lookfor: "b",
			expect:  true,
		},
		{
			name: "false",
			all: []string{
				"a", "b", "c",
			},
			lookfor: "d",
			expect:  false,
		},
	}
	for _, c := range cases {
		if get := HasFinalizer(c.all, c.lookfor); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestRemoveFinalizer(t *testing.T) {
	type testCase struct {
		name     string
		all      []string
		toRemove string
		expect   []string
	}

	cases := []testCase{
		{
			name: "removed",
			all: []string{
				"a", "b", "c",
			},
			toRemove: "a",
			expect: []string{
				"b", "c",
			},
		},
		{
			name: "not-changed",
			all: []string{
				"a", "b", "c",
			},
			toRemove: "d",
			expect: []string{
				"a", "b", "c",
			},
		},
	}
	for _, c := range cases {
		get := RemoveFinalizer(c.all, c.toRemove)
		if len(get) != len(c.expect) {
			t.Fatalf("error len")
		}
		for i := range get {
			if get[i] != c.expect[i] {
				t.Fatalf("different value, index %d", i)
			}
		}
	}
}

func TestNamespacedNameKeyFunc(t *testing.T) {
	type testCase struct {
		name   string
		ns     string
		n      string
		expect string
	}

	cases := []testCase{
		{
			name:   "has-namespace",
			ns:     "test",
			n:      "name",
			expect: "test/name",
		},
		{
			name:   "no-namespace",
			n:      "name",
			expect: "name",
		},
	}

	for _, c := range cases {
		if get := NamespacedNameKeyFunc(c.ns, c.n); get != c.expect {
			t.Fatalf("case %s: expect %s, get %s", c.name, c.expect, get)
		}
	}
}

func TestGetDuration(t *testing.T) {
	type testCase struct {
		name         string
		cfg          *lbcfapi.Duration
		defaultValue time.Duration
		expect       time.Duration
	}
	cases := []testCase{
		{
			name:         "nil-cfg",
			cfg:          nil,
			defaultValue: DefaultEnsurePeriod,
			expect:       DefaultEnsurePeriod,
		},
		{
			name: "has-cfg",
			cfg: &lbcfapi.Duration{
				Duration: 3 * time.Second,
			},
			defaultValue: DefaultEnsurePeriod,
			expect:       3 * time.Second,
		},
	}

	for _, c := range cases {
		if get := GetDuration(c.cfg, c.defaultValue); get != c.expect {
			t.Fatalf("case %s, expect %s, get %s", c.name, c.expect.String(), get.String())
		}
	}
}

func TestMapEqual(t *testing.T) {
	type tc struct {
		name   string
		m1     map[string]string
		m2     map[string]string
		expect bool
	}

	cases := []tc{
		{
			name: "equal",
			m1: map[string]string{
				"k1": "v1",
			},
			m2: map[string]string{
				"k1": "v1",
			},
			expect: true,
		}, {
			name: "not-equal1",
			m1: map[string]string{
				"k1": "v1",
			},
			m2: map[string]string{
				"k1": "v11",
			},
			expect: false,
		}, {
			name: "not-euqal2",
			m1: map[string]string{
				"k1": "v1",
			},
			m2: map[string]string{
				"k11": "v1",
			},
			expect: false,
		}, {
			name: "not-equal3",
			m1: map[string]string{
				"k1": "v1",
			},
			m2: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
			expect: false,
		},
	}

	for _, c := range cases {
		if get := MapEqual(c.m1, c.m2); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestResyncPolicyEqual(t *testing.T) {
	type tc struct {
		name   string
		a      *lbcfapi.EnsurePolicyConfig
		b      *lbcfapi.EnsurePolicyConfig
		expect bool
	}

	cases := []tc{
		{
			name:   "nil-equal",
			a:      nil,
			b:      nil,
			expect: true,
		},
		{
			name: "IfNotSucc-equal",
			a: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyIfNotSucc,
			},
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyIfNotSucc,
				MinPeriod: &lbcfapi.Duration{
					Duration: time.Second,
				},
			},
			expect: true,
		},
		{
			name: "always-equal",
			a: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: 5 * time.Second,
				},
			},
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: 5 * time.Second,
				},
			},
			expect: true,
		},
		{
			name: "always-equal2",
			a: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
			},
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
			},
			expect: true,
		},
		{
			name: "always-period-not-equal",
			a: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
			},
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: DefaultEnsurePeriod,
				},
			},
			expect: false,
		}, {
			name: "always-period-not-equal2",
			a: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: 2 * DefaultEnsurePeriod,
				},
			},
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: DefaultEnsurePeriod,
				},
			},
			expect: false,
		},
		{
			name: "policy-not-equal",
			a: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyIfNotSucc,
			},
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: DefaultEnsurePeriod,
				},
			},
			expect: false,
		}, {
			name: "nil-not-equal",
			a:    nil,
			b: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyAlways,
				MinPeriod: &lbcfapi.Duration{
					Duration: DefaultEnsurePeriod,
				},
			},
			expect: false,
		},
	}

	for _, c := range cases {
		if get := EnsurePolicyEqual(c.a, c.b); get != c.expect {
			t.Fatalf("case %s, expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestMakeBackendName(t *testing.T) {
	lbName := "lb"
	groupName := "group"
	podName := "pod"
	p := "tcp"
	p2 := "Tcp"
	p3 := "TCP"
	p4 := "udp"
	equals := []lbcfapi.PortSelector{
		{
			PortNumber: 12324,
			Protocol:   &p,
		},
		{
			PortNumber: 12324,
		},
		{
			PortNumber: 12324,
			Protocol:   &p2,
		},
		{
			PortNumber: 12324,
			Protocol:   &p3,
		},
	}
	notEqual := lbcfapi.PortSelector{
		PortNumber: 12324,
		Protocol:   &p4,
	}
	ne := MakeBackendName(lbName, groupName, podName, notEqual)
	lastOne := ""
	for i := range equals {
		n := MakeBackendName(lbName, groupName, podName, equals[i])
		if i == 0 {
			lastOne = n
		} else {
			if lastOne != n {
				t.Fatalf("expect %s, get %s", lastOne, n)
			}
		}
		if n == ne {
			t.Fatalf("should not equal")
		}
	}
}

func TestMakeBackendLabels(t *testing.T) {
	driverName := "driver"
	lbName := "lb"
	groupName := "group"
	podName := "pod-0"
	svcName := "my-svc"
	statidAddr := "1.1.1.1"
	get := MakeBackendLabels(driverName, lbName, groupName, svcName, podName, statidAddr)
	expect := map[string]string{
		lbcfapi.LabelDriverName:  driverName,
		lbcfapi.LabelLBName:      lbName,
		lbcfapi.LabelGroupName:   groupName,
		lbcfapi.LabelPodName:     podName,
		lbcfapi.LabelServiceName: svcName,
		lbcfapi.LabelStaticAddr:  statidAddr,
	}
	if !MapEqual(get, expect) {
		t.Fatalf("expect %v, get %v", expect, get)
	}
}

func TestIterateBackends(t *testing.T) {
	i := 0
	increase := func(*lbcfapi.BackendRecord) error {
		i++
		return nil
	}
	backends := []*lbcfapi.BackendRecord{
		{},
		{},
		{},
		{},
	}
	err := IterateBackends(backends, increase)
	if err != nil {
		t.Fatalf("expect nil err, get %v", err)
	}
	if i != len(backends) {
		t.Fatalf("expect %d, get %d", len(backends), i)
	}

	allErr := func(record *lbcfapi.BackendRecord) error {
		return fmt.Errorf("fake error")
	}
	err = IterateBackends(backends, allErr)
	el := err.(ErrorList)
	if len(el) != len(backends) {
		t.Fatalf("wrong len")
	}
	for _, e := range el {
		if e.Error() != "fake error" {
			t.Fatalf("wrong err.Error() %s", e.Error())
		}
	}
}

func TestFilterPods(t *testing.T) {
	allPods := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "selected 1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "selected 2",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ignored",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "selected 3",
			},
		},
	}
	filterByName := func(pod *v1.Pod) bool {
		if strings.HasPrefix(pod.Name, "selected") {
			return true
		}
		return false
	}

	get := FilterPods(allPods, filterByName)
	if len(get) != 3 {
		t.Fatalf("expect 3, get %d", len(get))
	}
	expectedSet := sets.NewString()
	expectedSet.Insert("selected 1", "selected 2", "selected 3")
	getNameSet := sets.NewString()
	for _, g := range get {
		if getNameSet.Has(g.Name) {
			t.Fatalf("already exist %s", g.Name)
		}
		getNameSet.Insert(g.Name)
	}
	if !expectedSet.Equal(getNameSet) {
		t.Fatalf("expect %v, get %v", expectedSet.List(), getNameSet.List())
	}
}

func TestIsPodMatchBackendGroup(t *testing.T) {
	type tc struct {
		name   string
		group  *lbcfapi.BackendGroup
		pod    *v1.Pod
		expect bool
	}

	cases := []tc{
		{
			name: "byName-match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						ByName: []string{
							"my-pod-0",
						},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pod-0",
				},
			},
			expect: true,
		},
		{
			name: "byName-not-match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						ByName: []string{
							"my-pod-0",
						},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pod-1",
				},
			},
			expect: false,
		},
		{
			name: "byLabel-match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
								"k2": "v2",
							},
						},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"k1": "v1",
						"k2": "v2",
						"k3": "v3",
					},
				},
			},
			expect: true,
		},
		{
			name: "byLabel-not-match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
								"k2": "v2",
							},
						},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"k1": "v1",
					},
				},
			},
			expect: false,
		},
		{
			name: "byLabel-except-not-match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
								"k2": "v2",
							},
							Except: []string{
								"my-pod-0",
							},
						},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pod-0",
					Labels: map[string]string{
						"k1": "v1",
						"k2": "v2",
					},
				},
			},
			expect: false,
		},
		{
			name: "namespace-not-match",
			group: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						ByName: []string{
							"my-pod-0",
						},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-pod-0",
					Namespace: "another-ns",
				},
			},
			expect: false,
		},
		{
			name: "non-pod-backend",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Static: []string{
						"1.1.1.1",
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pod-0",
				},
			},
			expect: false,
		},
	}

	for _, c := range cases {
		if get := IsPodMatchBackendGroup(c.group, c.pod); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestIsLBMatchBackendGroup(t *testing.T) {
	type tc struct {
		name   string
		group  *lbcfapi.BackendGroup
		lb     *lbcfapi.LoadBalancer
		expect bool
	}

	cases := []tc{
		{
			name: "match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LBName: "my-lb",
				},
			},
			lb: &lbcfapi.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-lb",
				},
			},
			expect: true,
		},
		{
			name: "name-not-match",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LBName: "my-lb",
				},
			},
			lb: &lbcfapi.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "another-lb",
				},
			},
			expect: false,
		},
		{
			name: "namespace-not-match",
			group: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LBName: "my-lb",
				},
			},
			lb: &lbcfapi.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-lb",
					Namespace: "another-namespace",
				},
			},
			expect: false,
		},
	}
	for _, c := range cases {
		if get := IsLBMatchBackendGroup(c.group, c.lb); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestCompareBackendRecords(t *testing.T) {
	expectAdd := &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expect-add",
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:   "lb",
			LBDriver: "driver",
			LBInfo: map[string]string{
				"lbID": "1234",
			},
			LBAttributes: map[string]string{
				"attr1": "v1",
			},
			PodBackendInfo: &lbcfapi.PodBackendRecord{
				Name: "my-pod-0",
				Port: lbcfapi.PortSelector{
					PortNumber: 8080,
				},
			},
			Parameters: map[string]string{},
		},
	}

	expectDelete := &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expect-delete",
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:   "lb",
			LBDriver: "driver",
			LBInfo: map[string]string{
				"lbID": "1234",
			},
			LBAttributes: map[string]string{
				"attr1": "v1",
			},
			PodBackendInfo: &lbcfapi.PodBackendRecord{
				Name: "my-pod-1",
				Port: lbcfapi.PortSelector{
					PortNumber: 8080,
				},
			},
			Parameters: map[string]string{},
		},
	}

	expectUpdate1 := &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: "update1",
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:   "lb",
			LBDriver: "driver",
			LBInfo: map[string]string{
				"lbID": "1234",
			},
			LBAttributes: map[string]string{
				"attr1": "v1",
			},
			PodBackendInfo: &lbcfapi.PodBackendRecord{
				Name: "my-pod-1",
				Port: lbcfapi.PortSelector{
					PortNumber: 8080,
				},
			},
			Parameters: map[string]string{
				"para1": "value1",
			},
		},
	}
	expectUpdate2 := expectUpdate1.DeepCopy()
	expectUpdate2.Name = "update2"

	expectUpdate3 := expectUpdate1.DeepCopy()
	expectUpdate3.Name = "update3"

	update1 := expectUpdate1.DeepCopy()
	update1.Spec.LBAttributes["update-attr"] = "value"

	update2 := expectUpdate2.DeepCopy()
	update2.Spec.Parameters["update-para"] = "value"

	update3 := expectUpdate3.DeepCopy()
	update3.Spec.EnsurePolicy = &lbcfapi.EnsurePolicyConfig{
		Policy: lbcfapi.PolicyAlways,
		MinPeriod: &lbcfapi.Duration{
			Duration: 30 * time.Second,
		},
	}

	expectSame := &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: "expect-same",
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:   "lb",
			LBDriver: "driver",
			LBInfo: map[string]string{
				"lbID": "1234",
			},
			LBAttributes: map[string]string{
				"attr1": "v1",
			},
			PodBackendInfo: &lbcfapi.PodBackendRecord{
				Name: "my-pod-0",
				Port: lbcfapi.PortSelector{
					PortNumber: 8080,
				},
			},
			Parameters: map[string]string{},
		},
	}
	expect := []*lbcfapi.BackendRecord{expectAdd, expectSame, expectUpdate1, expectUpdate2, expectUpdate3}
	have := []*lbcfapi.BackendRecord{expectDelete, expectSame, update1, update2, update3}

	getAdd, getUpdate, getDelete := CompareBackendRecords(expect, have)
	if len(getAdd) != 1 {
		t.Fatalf("expect 1, get %d", len(getAdd))
	} else if getAdd[0] != expectAdd {
		t.Fatalf("expectAdd %+v, getAdd %+v", expectAdd, getAdd)
	}

	if len(getUpdate) != 3 {
		for _, g := range getUpdate {
			t.Log(g.Name)
		}
		t.Fatalf("expect update 3, get %d", len(getUpdate))
	}

	if len(getDelete) != 1 {
		t.Fatalf("expect 1, get %d", len(getDelete))
	} else if getDelete[0] != expectDelete {
		t.Fatalf("expectDelete %+v, getDelete %+v", expectDelete, getDelete)
	}
}

func TestSyncResult(t *testing.T) {
	succ := SuccResult()
	empty := SyncResult{}
	if *succ != empty {
		t.Fatalf("expect %+v, get %+v", empty, *succ)
	}

	if !ErrorResult(fmt.Errorf("fake error")).IsError() {
		t.Fatalf("expect error")
	}

	if !FailResult(5 * time.Second).IsFailed() {
		t.Fatalf("expect fail")
	}

	if !AsyncResult(5 * time.Second).IsRunning() {
		t.Fatalf("expect async")
	}

	if !PeriodicResult(5 * time.Second).IsPeriodic() {
		t.Fatalf("expect periodic")
	}
}

func TestMakeFinalizerPatch(t *testing.T) {
	type testCase struct {
		name          string
		finalizer     string
		expectedPatch string
	}

	cases := []testCase{
		{
			name:          "lb-patch",
			finalizer:     lbcfapi.FinalizerDeleteLB,
			expectedPatch: `[{"op":"add","path":"/metadata/finalizers/-","value":"lbcf.tke.cloud.tencent.com/delete-load-loadbalancer"}]`,
		},
		{
			name:          "backend-patch",
			finalizer:     lbcfapi.FinalizerDeregisterBackend,
			expectedPatch: `[{"op":"add","path":"/metadata/finalizers/-","value":"lbcf.tke.cloud.tencent.com/deregister-backend"}]`,
		},
		{
			name:          "backend-group-patch",
			finalizer:     lbcfapi.FinalizerDeregisterBackendGroup,
			expectedPatch: `[{"op":"add","path":"/metadata/finalizers/-","value":"lbcf.tke.cloud.tencent.com/deregister-backend-group"}]`,
		},
	}
	for _, c := range cases {
		if get := MakeFinalizerPatch(false, c.finalizer); string(get) != c.expectedPatch {
			t.Fatalf("case %s: expect %s, get %s", c.name, c.expectedPatch, string(get))
		}
	}
}

func TestLBNeedEnsure(t *testing.T) {
	type testCase struct {
		name   string
		lb     *lbcfapi.LoadBalancer
		expect bool
	}
	cases := []testCase{
		{
			name: "need-not-ensured-yet",
			lb: &lbcfapi.LoadBalancer{
				Status: lbcfapi.LoadBalancerStatus{
					Conditions: []lbcfapi.LoadBalancerCondition{
						{
							Type:   lbcfapi.LBEnsured,
							Status: lbcfapi.ConditionFalse,
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-periodic-ensure",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
					},
				},
				Status: lbcfapi.LoadBalancerStatus{
					Conditions: []lbcfapi.LoadBalancerCondition{
						{
							Type:   lbcfapi.LBEnsured,
							Status: lbcfapi.ConditionTrue,
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "no-need-policy-ifnotsucc",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyIfNotSucc,
					},
				},
				Status: lbcfapi.LoadBalancerStatus{
					Conditions: []lbcfapi.LoadBalancerCondition{
						{
							Type:   lbcfapi.LBEnsured,
							Status: lbcfapi.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "no-need-empty-policy",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{},
				Status: lbcfapi.LoadBalancerStatus{
					Conditions: []lbcfapi.LoadBalancerCondition{
						{
							Type:   lbcfapi.LBEnsured,
							Status: lbcfapi.ConditionTrue,
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		if get := LBNeedEnsure(c.lb); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestLoadBalancerNonStatusUpdated(t *testing.T) {
	type testCase struct {
		name   string
		old    *lbcfapi.LoadBalancer
		cur    *lbcfapi.LoadBalancer
		expect bool
	}
	cases := []testCase{
		{
			name: "need-attributes-updated",
			old: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{},
			},
			cur: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					Attributes: map[string]string{
						"k1": "v1",
					},
				},
			},
			expect: true,
		},
		{
			name: "need-ensurePolicy-updated",
			old: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{},
			},
			cur: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyIfNotSucc,
					},
				},
			},
			expect: true,
		},
		{
			name: "no-need-only-status-updated",
			old: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{},
			},
			cur: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{},
				Status: lbcfapi.LoadBalancerStatus{
					LBInfo: map[string]string{
						"k1": "v1",
					},
				},
			},
		},
	}
	for _, c := range cases {
		if get := LoadBalancerNonStatusUpdated(c.old, c.cur); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestBackendGroupNonStatusUpdated(t *testing.T) {
	type testCase struct {
		name   string
		old    *lbcfapi.BackendGroup
		cur    *lbcfapi.BackendGroup
		expect bool
	}
	udp := "udp"
	cases := []testCase{
		{
			name: "need-pod-port",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 8080,
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-pod-port-protocol",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
							Protocol:   &udp,
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-pod-label-selector",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
						},
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
								"k2": "v2",
							},
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-pod-label-except",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
						},
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
							Except: []string{
								"pod-0",
							},
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-pod-names",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
						ByName: []string{},
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: 80,
						},
						ByName: []string{
							"pod-0",
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-static-changed",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Static: []string{},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Static: []string{
						"pod-0",
					},
				},
			},
			expect: true,
		},
		{
			name: "need-parameters-changed",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
			expect: true,
		},
		{
			name: "need-ensurePolicy-changed",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyIfNotSucc,
					},
				},
			},
			expect: true,
		},
		{
			name: "no-need-only-status-changed",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{},
				Status: lbcfapi.BackendGroupStatus{
					Backends:           2,
					RegisteredBackends: 2,
				},
			},
		},
	}
	for _, c := range cases {
		if get := BackendGroupNonStatusUpdated(c.old, c.cur); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestBackendNeedEnsure(t *testing.T) {
	type testCase struct {
		name    string
		backend *lbcfapi.BackendRecord
		expect  bool
	}
	cases := []testCase{
		{
			name: "need-not-ensured-yet",
			backend: &lbcfapi.BackendRecord{
				Status: lbcfapi.BackendRecordStatus{},
			},
			expect: true,
		},
		{
			name: "need-not-ensured-yet-2",
			backend: &lbcfapi.BackendRecord{
				Status: lbcfapi.BackendRecordStatus{
					Conditions: []lbcfapi.BackendRecordCondition{
						{
							Type:   lbcfapi.BackendRegistered,
							Status: lbcfapi.ConditionFalse,
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "need-periodic-ensure",
			backend: &lbcfapi.BackendRecord{
				Spec: lbcfapi.BackendRecordSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
					},
				},
				Status: lbcfapi.BackendRecordStatus{
					Conditions: []lbcfapi.BackendRecordCondition{
						{
							Type:   lbcfapi.BackendRegistered,
							Status: lbcfapi.ConditionTrue,
						},
					},
				},
			},
			expect: true,
		},
		{
			name: "no-need",
			backend: &lbcfapi.BackendRecord{
				Spec: lbcfapi.BackendRecordSpec{},
				Status: lbcfapi.BackendRecordStatus{
					Conditions: []lbcfapi.BackendRecordCondition{
						{
							Type:   lbcfapi.BackendRegistered,
							Status: lbcfapi.ConditionTrue,
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		if get := BackendNeedEnsure(c.backend); get != c.expect {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expect, get)
		}
	}
}

func TestFilterBackendGroup(t *testing.T) {
	all := []*lbcfapi.BackendGroup{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "selected-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ignored",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "selected-2",
			},
		},
	}
	filterFunc := func(group *lbcfapi.BackendGroup) bool {
		if strings.HasPrefix(group.Name, "selected") {
			return true
		}
		return false
	}
	expect := sets.NewString("selected-1", "selected-2")
	result := FilterBackendGroup(all, filterFunc)
	get := sets.NewString()
	for _, r := range result {
		get.Insert(r.Name)
	}
	if !get.Equal(expect) {
		t.Fatalf("expect: %v, get %v", expect.List(), get.List())
	}
}
