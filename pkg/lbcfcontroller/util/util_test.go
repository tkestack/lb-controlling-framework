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
			t.Errorf("pod should bind, but return false, pod: %+v", pod)
		}
	}
	for _, pod := range shouldNotBind {
		if PodAvailable(&pod) {
			t.Errorf("pod should not bind, but return true, pod: %+v", pod)
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
			t.Errorf("expect created, index: %d", i)
		}
	}
	for i, lb := range notCreated {
		if LBCreated(lb) {
			t.Errorf("expect not-created, index: %d", i)
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
			t.Errorf("case: %s. wrong length, expect: %d, get: %d", tc.name, len(tc.expect.Conditions), len(tc.status.Conditions))
		}
		for _, c := range tc.expect.Conditions {
			get := getLBCondition(tc.status, c.Type)
			if get == nil {
				t.Errorf("case: %s. not found", tc.name)
				continue
			}
			if *get != c {
				t.Errorf("case: %s, condition not equal, expect: %+v, get: %+v", tc.name, c, *get)
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
			t.Errorf("case: %s. wrong length, expect: %d, get: %d", tc.name, len(tc.expect.Conditions), len(tc.status.Conditions))
		}
		for _, c := range tc.expect.Conditions {
			found := false
			for i := range tc.status.Conditions {
				if tc.status.Conditions[i].Type == c.Type {
					found = true
					if tc.status.Conditions[i] != c {
						t.Errorf("case: %s, condition not equal, expect: %+v, get: %+v", tc.name, c, tc.status.Conditions[i])
					}
				}
			}
			if !found {
				t.Errorf("case: %s. not found", tc.name)
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
			t.Errorf("case %s: expect type %s, get %s", c.name, c.backendType, get)
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
			t.Errorf("case %s: expect %s, get %s", c.name, c.expectNamespace, get)
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
			t.Errorf("case %s: expect %v, get %v", c.name, c.expect, get)
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
			t.Errorf("case %s: expect %v, get %v", c.name, c.expected, get)
		}
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
			expectedPatch: `[{"op":"add","path":"/metadata/finalizers/-","value":["lbcf.tke.cloud.tencent.com/delete-load-loadbalancer"]}]`,
		},
		{
			name:          "backend-patch",
			finalizer:     lbcfapi.FinalizerDeregisterBackend,
			expectedPatch: `[{"op":"add","path":"/metadata/finalizers/-","value":["lbcf.tke.cloud.tencent.com/deregister-backend"]}]`,
		},
		{
			name:          "backend-group-patch",
			finalizer:     lbcfapi.FinalizerDeregisterBackendGroup,
			expectedPatch: `[{"op":"add","path":"/metadata/finalizers/-","value":["lbcf.tke.cloud.tencent.com/deregister-backend-group"]}]`,
		},
	}
	for _, c := range cases {
		if get := MakeFinalizerPatch(c.finalizer); string(get) != c.expectedPatch {
			t.Errorf("case %s: expect %s, get %s", c.name, c.expectedPatch, string(get))
		}
	}
}
