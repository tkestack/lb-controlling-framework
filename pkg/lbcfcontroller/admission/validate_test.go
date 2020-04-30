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

package admission

import (
	"testing"
	"time"

	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateLoadBalancerDriver(t *testing.T) {
	type testCast struct {
		name        string
		driver      *lbcfapi.LoadBalancerDriver
		expectValid bool
	}
	cases := []testCast{
		{
			name: "valid-sys-driver",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-test-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{
							Name: webhooks.ValidateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.CreateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-user-driver",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{
							Name: webhooks.ValidateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.CreateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "invalid-sys-driver",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-name",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
		},
		{
			name: "invalid-sys-driver-2",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "non-kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
		},
		{
			name: "invalid-driver-type",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: "invalid-type",
					URL:        "http://1.1.1.1:80",
				},
			},
		},
		{
			name: "invalid-url",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "1.1.1.1:80",
				},
			},
		},
		{
			name: "invalid-webhook-config-no-name",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{},
					},
				},
			},
		},
		{
			name: "invalid-webhook-config-timeout-too-long",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{
							Name: webhooks.ValidateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 100 * time.Hour,
							},
						},
					},
				},
			},
		},
		{
			name: "invalid-webhook-config-duplicate-name",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{
							Name: webhooks.ValidateLoadBalancer,
						},
						{
							Name: webhooks.ValidateLoadBalancer,
						},
					},
				},
			},
		},
		{
			name: "invalid-webhook-config-unsupported-name",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lbcf-driver",
					Namespace: "kube-system",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{
							Name: "not-supported-name",
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		err := ValidateLoadBalancerDriver(c.driver)
		if c.expectValid && len(err) > 0 {
			t.Fatalf("case %s, expect valid, get error: %v", c.name, err.ToAggregate().Error())
		} else if !c.expectValid && len(err) == 0 {
			t.Fatalf("case %s, expect invalid, get valid", c.name)
		}
	}
}

func TestValidateLoadBalancer(t *testing.T) {
	type testCase struct {
		name        string
		lb          *lbcfapi.LoadBalancer
		expectValid bool
	}
	cases := []testCase{
		{
			name: "valid",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-2",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-3",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyIfNotSucc,
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-4",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "invalid-empty-lbDriver",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "",
				},
			},
		},
		{
			name: "invalid-ensurePolicy",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyIfNotSucc,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-ensurePolicy-too-short",
			lb: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 29 * time.Second,
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		err := ValidateLoadBalancer(c.lb)
		if c.expectValid && len(err) > 0 {
			t.Fatalf("case %s, expect valid, get error: %v", c.name, err.ToAggregate().Error())
		} else if !c.expectValid && len(err) == 0 {
			t.Fatalf("case %s, expect invalid, get valid", c.name)
		}
	}
}

func TestValidateBackendGroup(t *testing.T) {
	type testCase struct {
		name        string
		group       *lbcfapi.BackendGroup
		expectValid bool
	}
	tcp := "TCP"
	udp := "UDP"
	invalid := "invalid"
	ifNotRunning := lbcfapi.DeregisterIfNotRunning
	cases := []testCase{
		{
			name: "valid-empty-group",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-pod-backend",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: tcp,
							},
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
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-pod-backend-2",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: udp,
							},
						},
						ByName: []string{
							"pod-1",
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-svc-backend",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Service: &lbcfapi.ServiceBackend{
						Name: "svc-name",
						Port: lbcfapi.PortSelector{
							Port:     80,
							Protocol: tcp,
						},
						NodeSelector: map[string]string{
							"k1": "v1",
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-static-backend",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Static: []string{
						"1.1.1.1:80",
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-deregisterPolicy",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Static: []string{
						"1.1.1.1:80",
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					DeregisterPolicy: &ifNotRunning,
				},
			},
			expectValid: true,
		},
		{
			name: "valid-deregisterPolicy-empty",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Static: []string{
						"1.1.1.1:80",
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "invalid-multi-backend-svc-pod",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: udp,
							},
						},
						ByName: []string{
							"pod-1",
						},
					},
					Service: &lbcfapi.ServiceBackend{
						Name: "svc-name",
						Port: lbcfapi.PortSelector{
							Port: 80,
						},
						NodeSelector: map[string]string{
							"k1": "v1",
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-multi-backend-svc-static",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Static: []string{
						"pod-0",
					},
					Service: &lbcfapi.ServiceBackend{
						Name: "svc-name",
						Port: lbcfapi.PortSelector{
							Port: 80,
						},
						NodeSelector: map[string]string{
							"k1": "v1",
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-multi-backend-pod-static",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: udp,
							},
						},
						ByName: []string{
							"pod-1",
						},
					},
					Static: []string{
						"pod-0",
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-port-selector",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: invalid,
							},
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
		},
		{
			name: "invalid-port-number",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     0,
								Protocol: tcp,
							},
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
		},
		{
			name: "invalid-pod-multi-by",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: invalid,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
							Except: []string{
								"pod-0",
							},
						},
						ByName: []string{
							"my-pod-1",
						},
					},
				},
			},
		},
		{
			name: "invalid-pod-no-by",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: invalid,
							},
						},
					},
				},
			},
		},
		{
			name: "invalid-ensurePolicy",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: udp,
							},
						},
						ByName: []string{
							"pod-1",
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyIfNotSucc,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-pod-backend-no-selector",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: tcp,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-pod-backend-invalid-label",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port:     80,
								Protocol: tcp,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"kayc./-jaj": "kayc./-jaj",
							},
							Except: []string{
								"pod-0",
							},
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "invalid-svc-backend-invalid-nodeSelector",
			group: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
					Service: &lbcfapi.ServiceBackend{
						Name: "svc-name",
						Port: lbcfapi.PortSelector{
							Port:     80,
							Protocol: tcp,
						},
						NodeSelector: map[string]string{
							"kayc./-jaj": "kayc./-jaj",
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 30 * time.Second,
						},
					},
				},
			},
		},
	}
	for _, c := range cases {
		err := ValidateBackendGroup(c.group)
		if c.expectValid && len(err) > 0 {
			t.Fatalf("case %s, expect valid, get error: %v", c.name, err.ToAggregate().Error())
		} else if !c.expectValid && len(err) == 0 {
			t.Fatalf("case %s, expect invalid, get valid", c.name)
		}
	}
}

func TestDriverUpdatedFieldsAllowed(t *testing.T) {
	type testCase struct {
		name        string
		old         *lbcfapi.LoadBalancerDriver
		cur         *lbcfapi.LoadBalancerDriver
		expectValid bool
	}
	cases := []testCase{
		{
			name: "valid-change-webhookconfig",
			old: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
			cur: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
					Webhooks: []lbcfapi.WebhookConfig{
						{
							Name: webhooks.ValidateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid-change-status",
			old: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
			cur: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
				Status: lbcfapi.LoadBalancerDriverStatus{
					Conditions: []lbcfapi.LoadBalancerDriverCondition{
						{
							Type:               lbcfapi.DriverAccepted,
							Status:             lbcfapi.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "invalid-change-url",
			old: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
			cur: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://2.2.2.2:80",
				},
			},
		},
		{
			name: "invalid-change-driverType",
			old: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
			cur: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string("invalid"),
					URL:        "http://1.1.1.1:80",
				},
			},
		},
	}
	for _, c := range cases {
		if get, _ := DriverUpdatedFieldsAllowed(c.cur, c.old); get != c.expectValid {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expectValid, get)
		}
	}
}

func TestLBUpdatedFieldsAllowed(t *testing.T) {
	type testCase struct {
		name        string
		old         *lbcfapi.LoadBalancer
		cur         *lbcfapi.LoadBalancer
		expectValid bool
	}

	cases := []testCase{
		{
			name: "valid-change",
			old: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
				},
			},
			cur: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a2": "v2",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{
							Duration: 1 * time.Minute,
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "invalid-change-lbSpec",
			old: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
				},
			},
			cur: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k2": "v2",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
				},
			},
		},
		{
			name: "invalid-change-lbDriver",
			old: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
				},
			},
			cur: &lbcfapi.LoadBalancer{
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver-2",
					LBSpec: map[string]string{
						"k1": "v1",
					},
					Attributes: map[string]string{
						"a1": "v1",
					},
				},
			},
		},
	}
	for _, c := range cases {
		if get, _ := LBUpdatedFieldsAllowed(c.cur, c.old); get != c.expectValid {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expectValid, get)
		}
	}
}

func TestBackendGroupUpdateFieldsAllowed(t *testing.T) {
	type testCase struct {
		name        string
		old         *lbcfapi.BackendGroup
		cur         *lbcfapi.BackendGroup
		expectValid bool
	}
	cases := []testCase{
		{
			name: "valid",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-loadbalancer"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port: 80,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-loadbalancer"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port: 8080,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k2": "v2",
							},
						},
					},
					Parameters: map[string]string{
						"p2": "v2",
					},
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy: lbcfapi.PolicyAlways,
					},
				},
			},
			expectValid: true,
		},
		{
			name:        "invalid-change-lbName",
			expectValid: true,
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-loadbalancer"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port: 80,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-loadbalancer-2"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port: 80,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
		},
		{
			name: "invalid-change-type",
			old: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-loadbalancer"},
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{
								Port: 80,
							},
						},
						ByLabel: &lbcfapi.SelectPodByLabel{
							Selector: map[string]string{
								"k1": "v1",
							},
						},
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
			cur: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-loadbalancer"},
					Static: []string{
						"pod-1",
					},
					Parameters: map[string]string{
						"p1": "v1",
					},
				},
			},
		},
	}
	for _, c := range cases {
		if get, _ := BackendGroupUpdateFieldsAllowed(c.cur, c.old); get != c.expectValid {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expectValid, get)
		}
	}
}
