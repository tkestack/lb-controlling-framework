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
	"encoding/json"
	"testing"
	"time"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	lbcflister "tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAdmitter_MutateLB(t *testing.T) {
	a := fakeAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})

	// case 1: create finalizers array
	lb := &lbcfapi.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{},
	}
	raw, _ := json.Marshal(lb)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	rsp := a.MutateLB(ar)
	if !rsp.Allowed {
		t.Fatalf("expect allow")
	} else if string(rsp.Patch) != `[{"op":"add","path":"/metadata/finalizers","value":["lbcf.tkestack.io/delete-load-loadbalancer"]}]` {
		t.Fatalf("wrong patch")
	}

	// case 2: append finalizers array
	lb = &lbcfapi.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{
				"finalizer1",
			},
		},
	}
	raw, _ = json.Marshal(lb)
	ar = &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	rsp = a.MutateLB(ar)
	if !rsp.Allowed {
		t.Fatalf("expect allow")
	} else if string(rsp.Patch) != `[{"op":"add","path":"/metadata/finalizers/-","value":"lbcf.tkestack.io/delete-load-loadbalancer"}]` {
		t.Fatalf("wrong patch")
	}

	// case 3: object decode error
	ar = &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: []byte("invalid json"),
			},
		},
	}
	rsp = a.MutateLB(ar)
	if rsp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_MutateDriver(t *testing.T) {
	a := fakeAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	driver := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "test",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			URL:        "http://test-driver.com",
		},
	}
	raw, _ := json.Marshal(driver)
	rsp := a.MutateDriver(&v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	})
	if !rsp.Allowed {
		t.Fatalf("expect always allow")
	}

	patch, err := jsonpatch.DecodePatch(rsp.Patch)
	if err != nil {
		t.Fatalf(err.Error())
	}
	modified, err := patch.Apply(raw)
	if err != nil {
		t.Fatalf(err.Error())
	}
	modifiedDriver := &lbcfapi.LoadBalancerDriver{}
	if err := json.Unmarshal(modified, modifiedDriver); err != nil {
		t.Fatalf(err.Error())
	}
	if len(modifiedDriver.Spec.Webhooks) != len(webhooks.KnownWebhooks) {
		t.Errorf("expect %d webhooks, get %d", len(webhooks.KnownWebhooks), len(modifiedDriver.Spec.Webhooks))
	}
	for known := range webhooks.KnownWebhooks {
		found := false
		for _, wh := range modifiedDriver.Spec.Webhooks {
			if wh.Name == known {
				found = true
				if wh.Timeout.Duration != defaultWebhookTimeout {
					t.Errorf("webhook %s expect timeout %s, get %s", wh.Name, defaultWebhookTimeout.String(), wh.Timeout.Duration.String())
				}
				break
			}
		}
		if !found {
			t.Errorf("webhook %s not found", known)
		}
	}

	// case 2: partially specified
	existWebhook := map[string]lbcfapi.WebhookConfig{
		webhooks.ValidateLoadBalancer: {
			Name: webhooks.ValidateLoadBalancer,
			Timeout: lbcfapi.Duration{
				Duration: 13 * time.Second},
		},
		webhooks.ValidateBackend: {
			Name: webhooks.ValidateBackend,
			Timeout: lbcfapi.Duration{
				Duration: 15 * time.Second,
			},
		},
	}
	var hooks []lbcfapi.WebhookConfig
	for _, exist := range existWebhook {
		hooks = append(hooks, exist)
	}

	driver = &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "test",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			URL:        "http://test-driver.com",
			Webhooks:   hooks,
		},
	}
	raw, _ = json.Marshal(driver)
	rsp = a.MutateDriver(&v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	})
	patch, err = jsonpatch.DecodePatch(rsp.Patch)
	if err != nil {
		t.Fatalf(err.Error())
	}
	modified, err = patch.Apply(raw)
	if err != nil {
		t.Fatalf(err.Error())
	}
	modifiedDriver = &lbcfapi.LoadBalancerDriver{}
	if err := json.Unmarshal(modified, modifiedDriver); err != nil {
		t.Fatalf(err.Error())
	}
	if len(modifiedDriver.Spec.Webhooks) != len(webhooks.KnownWebhooks) {
		t.Errorf("expect %d webhooks, get %d", len(webhooks.KnownWebhooks), len(modifiedDriver.Spec.Webhooks))
	}

	for known := range webhooks.KnownWebhooks {
		found := false
		for _, wh := range modifiedDriver.Spec.Webhooks {
			if wh.Name == known {
				found = true
				if existTimeout, ok := existWebhook[wh.Name]; ok {
					if wh.Timeout.Duration != existTimeout.Timeout.Duration {
						t.Errorf("webhook %s expect timeout %s, get %s", wh.Name, existTimeout.Timeout.Duration.String(), wh.Timeout.Duration.String())
					}
				} else {
					if wh.Timeout.Duration != defaultWebhookTimeout {
						t.Errorf("webhook %s expect timeout %s, get %s", wh.Name, defaultWebhookTimeout.String(), wh.Timeout.Duration.String())
					}
				}
				break
			}
		}
		if !found {
			t.Errorf("webhook %s not found", known)
		}
	}
}

func TestAdmitter_MutateBackendGroupLabels(t *testing.T) {
	a := fakeAdmitter(
		&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{},
		nil,
		&alwaysSuccBackendLister{},
		&fakeSuccInvoker{})
	type testCase struct {
		name           string
		bg             *lbcfapi.BackendGroup
		expectedLabels map[string]string
	}
	cases := []testCase{
		{
			name: "add labels on bg without label",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg-without-labels",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb", "test-lb2"},
				},
			},
			expectedLabels: map[string]string{
				lbcfapi.LabelLBName:                    "test-lb",
				lbcfapi.LabelLBNamePrefix + "test-lb":  "True",
				lbcfapi.LabelLBNamePrefix + "test-lb2": "True",
			},
		},
		{
			name: "lbName set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LBName: toStringPtr("test-lb"),
				},
			},
			expectedLabels: map[string]string{
				lbcfapi.LabelLBName:                   "test-lb",
				lbcfapi.LabelLBNamePrefix + "test-lb": "True",
			},
		},
		{
			name: "lbName merged",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LBName:        toStringPtr("test-lb"),
					LoadBalancers: []string{"test-lb", "test-lb2"},
				},
			},
			expectedLabels: map[string]string{
				lbcfapi.LabelLBName:                    "test-lb",
				lbcfapi.LabelLBNamePrefix + "test-lb":  "True",
				lbcfapi.LabelLBNamePrefix + "test-lb2": "True",
			},
		},
		{
			name: "keep user specified labels",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg-with-labels",
					Labels: map[string]string{
						"user-label": "user-label-value",
					},
				},
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb", "test-lb2"},
				},
			},
			expectedLabels: map[string]string{
				"user-label":                           "user-label-value",
				lbcfapi.LabelLBName:                    "test-lb",
				lbcfapi.LabelLBNamePrefix + "test-lb":  "True",
				lbcfapi.LabelLBNamePrefix + "test-lb2": "True",
			},
		},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(c.bg)
		rsp := a.MutateBackendGroup(&v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: raw,
				},
			},
		})
		if !rsp.Allowed {
			t.Fatalf("expect always allow")
		}
		patch, err := jsonpatch.DecodePatch(rsp.Patch)
		if err != nil {
			t.Fatalf(err.Error())
		}
		modified, err := patch.Apply(raw)
		if err != nil {
			t.Fatalf("name:%s, err:%v", c.name, err.Error())
		}
		modifiedGroup := &lbcfapi.BackendGroup{}
		if err := json.Unmarshal(modified, modifiedGroup); err != nil {
			t.Fatalf(err.Error())
		}

		// check labels
		if len(modifiedGroup.Labels) != len(c.expectedLabels) {
			t.Fatalf("case: %s, expect %d, get %d", c.name, len(c.expectedLabels), len(modifiedGroup.Labels))
		}
		for ek, ev := range c.expectedLabels {
			if v, ok := modifiedGroup.Labels[ek]; !ok {
				t.Fatalf("missing label %s: %s", ek, ev)
			} else if v != ev {
				t.Fatalf("expected label value %s, get %s", ev, v)
			}
		}
	}
}

func TestAdmitter_MutateBackendGroupConvertLoadBalancers(t *testing.T) {
	a := fakeAdmitter(
		&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{},
		nil,
		&alwaysSuccBackendLister{},
		&fakeSuccInvoker{})
	type testCase struct {
		name                  string
		bg                    *lbcfapi.BackendGroup
		expectedLoadBalancers []string
	}
	cases := []testCase{
		{
			name: "lbName set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bg",
					Namespace: "test",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LBName: toStringPtr("test-lb"),
				},
			},
			expectedLoadBalancers: []string{"test-lb"},
		},
		{
			name: "loadBalancers set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bg",
					Namespace: "test",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LoadBalancers: []string{"test-lb"},
				},
			},
			expectedLoadBalancers: []string{"test-lb"},
		},
		{
			name: "both lbName and loadBalancers set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bg",
					Namespace: "test",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LBName:        toStringPtr("test-lb"),
					LoadBalancers: []string{"test-lb2"},
				},
			},
			expectedLoadBalancers: []string{"test-lb", "test-lb2"},
		},
		{
			name: "lbName and loadBalancers need merge",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bg",
					Namespace: "test",
				},
				Spec: lbcfapi.BackendGroupSpec{
					LBName:        toStringPtr("test-lb2"),
					LoadBalancers: []string{"test-lb2", "test-lb3"},
				},
			},
			expectedLoadBalancers: []string{"test-lb2", "test-lb3"},
		},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(c.bg)
		rsp := a.MutateBackendGroup(&v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: raw,
				},
			},
		})
		if !rsp.Allowed {
			t.Fatalf("expect always allow")
		}
		patch, err := jsonpatch.DecodePatch(rsp.Patch)
		if err != nil {
			t.Fatalf(err.Error())
		}
		modified, err := patch.Apply(raw)
		if err != nil {
			t.Fatalf("name:%s, err:%v", c.name, err.Error())
		}
		modifiedGroup := &lbcfapi.BackendGroup{}
		if err := json.Unmarshal(modified, modifiedGroup); err != nil {
			t.Fatalf(err.Error())
		}
		if modifiedGroup.Spec.LBName != nil {
			t.Fatalf("case %s: spec.lbName should be empty", c.name)
		}
		if len(modifiedGroup.Spec.LoadBalancers) != len(c.expectedLoadBalancers) {
			t.Fatalf("case %s: expect %d, get %d",
				c.name, len(c.expectedLoadBalancers), len(modifiedGroup.Spec.LoadBalancers))
		}
		for _, expect := range c.expectedLoadBalancers {
			found := false
			for _, get := range modifiedGroup.Spec.LoadBalancers {
				if expect == get {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("case %s: %s missing", c.name, expect)
			}
		}
	}
}

func TestAdmitter_MutateBackendGroupConvertPortSelector(t *testing.T) {
	a := fakeAdmitter(
		&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{},
		nil,
		&alwaysSuccBackendLister{},
		&fakeSuccInvoker{})
	type testCase struct {
		name          string
		bg            *lbcfapi.BackendGroup
		expectedPorts []*lbcfapi.PortSelector
	}
	cases := []testCase{
		{
			name: "spec.pods.port set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: &lbcfapi.PortSelector{
							PortNumber: int32Ptr(80),
						},
					},
				},
			},
			expectedPorts: []*lbcfapi.PortSelector{
				{
					Port:     80,
					Protocol: "TCP",
				},
			},
		},
		{
			name: "spec.pods.ports set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Ports: []lbcfapi.PortSelector{
							{PortNumber: int32Ptr(70)},
							{Port: 80},
							{Port: 90, Protocol: "UDP"},
						},
					},
				},
			},
			expectedPorts: []*lbcfapi.PortSelector{
				{
					Port:     70,
					Protocol: "TCP",
				},
				{
					Port:     80,
					Protocol: "TCP",
				},
				{
					Port:     90,
					Protocol: "UDP",
				},
			},
		},
		{
			name: "both spec.pods.port and ports set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					Pods: &lbcfapi.PodBackend{
						Port: &lbcfapi.PortSelector{
							PortNumber: int32Ptr(80),
						},
						Ports: []lbcfapi.PortSelector{
							{
								Port:     90,
								Protocol: "UDP",
							},
						},
					},
				},
			},
			expectedPorts: []*lbcfapi.PortSelector{
				{
					Port:     80,
					Protocol: "TCP",
				},
				{
					Port:     90,
					Protocol: "UDP",
				},
			},
		},
		{
			name: "spec.service.port.portNumber set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					Service: &lbcfapi.ServiceBackend{
						Port: lbcfapi.PortSelector{
							PortNumber: int32Ptr(80),
						},
					},
				},
			},
			expectedPorts: []*lbcfapi.PortSelector{
				{
					Port:     80,
					Protocol: "TCP",
				},
			},
		},
		{
			name: "spec.service.port.protocol not set",
			bg: &lbcfapi.BackendGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bg",
				},
				Spec: lbcfapi.BackendGroupSpec{
					Service: &lbcfapi.ServiceBackend{
						Port: lbcfapi.PortSelector{
							Port: 80,
						},
					},
				},
			},
			expectedPorts: []*lbcfapi.PortSelector{
				{
					Port:     80,
					Protocol: "TCP",
				},
			},
		},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(c.bg)
		rsp := a.MutateBackendGroup(&v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: raw,
				},
			},
		})
		if !rsp.Allowed {
			t.Fatalf("expect always allow")
		}
		patch, err := jsonpatch.DecodePatch(rsp.Patch)
		if err != nil {
			t.Fatalf(err.Error())
		}
		modified, err := patch.Apply(raw)
		if err != nil {
			t.Fatalf("name:%s, err:%v", c.name, err.Error())
		}
		modifiedGroup := &lbcfapi.BackendGroup{}
		if err := json.Unmarshal(modified, modifiedGroup); err != nil {
			t.Fatalf(err.Error())
		}
		if c.bg.Spec.Pods != nil {
			if modifiedGroup.Spec.Pods.Port != nil {
				t.Fatalf("case %s: port not nil", c.name)
			}
			if len(modifiedGroup.Spec.Pods.Ports) != len(c.expectedPorts) {
				t.Fatalf("case %s: expect %d, get %d",
					c.name, len(c.expectedPorts), len(modifiedGroup.Spec.Pods.Ports))
			}
			b, _ := json.MarshalIndent(modifiedGroup.Spec.Pods, "", "  ")
			t.Logf("%s", string(b))
			for _, expect := range c.expectedPorts {
				found := false
				for _, get := range modifiedGroup.Spec.Pods.Ports {
					if get.PortNumber != nil {
						t.Fatalf("case %s: portNumber not nil", c.name)
					}
					if expect.Port == get.Port && expect.Protocol == get.Protocol {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("case %s: %#v missiong", c.name, expect)
				}
			}
		} else if c.bg.Spec.Service != nil {
			b, _ := json.MarshalIndent(modifiedGroup.Spec.Service, "", "  ")
			t.Logf("%s", string(b))
			if modifiedGroup.Spec.Service.Port.PortNumber != nil {
				t.Fatalf("case %s: service.port.portNumber not nil", c.name)
			}
			get := modifiedGroup.Spec.Service.Port
			expect := c.expectedPorts[0]
			if get.Port != expect.Port || get.Protocol != expect.Protocol {
				t.Fatalf("case %s: expect %#v, get %#v", c.name, expect, get)
			}
		}
	}
}

func TestAdmitter_MutateBackendGroupDeregisterPolicy(t *testing.T) {
	a := fakeAdmitter(
		&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{},
		nil,
		&alwaysSuccBackendLister{},
		&fakeSuccInvoker{})
	type testCase struct {
		name           string
		bg             *lbcfapi.BackendGroup
		expectedPolicy lbcfapi.DeregPolicy
	}
	ifNotRunning := lbcfapi.DeregisterIfNotRunning
	cases := []testCase{
		{
			name:           "set default policy",
			bg:             &lbcfapi.BackendGroup{},
			expectedPolicy: lbcfapi.DeregisterIfNotReady,
		},
		{
			name: "user specified policy not changed",
			bg: &lbcfapi.BackendGroup{
				Spec: lbcfapi.BackendGroupSpec{
					DeregisterPolicy: &ifNotRunning,
				},
			},
			expectedPolicy: lbcfapi.DeregisterIfNotRunning,
		},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(c.bg)
		rsp := a.MutateBackendGroup(&v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: raw,
				},
			},
		})
		if !rsp.Allowed {
			t.Fatalf("expect always allow")
		}
		patch, err := jsonpatch.DecodePatch(rsp.Patch)
		if err != nil {
			t.Fatalf(err.Error())
		}
		modified, err := patch.Apply(raw)
		if err != nil {
			t.Fatalf("name:%s, err:%v", c.name, err.Error())
		}
		modifiedGroup := &lbcfapi.BackendGroup{}
		if err := json.Unmarshal(modified, modifiedGroup); err != nil {
			t.Fatalf(err.Error())
		}
		if modifiedGroup.Spec.DeregisterPolicy == nil {
			t.Fatalf("should be set")
		} else if *modifiedGroup.Spec.DeregisterPolicy != c.expectedPolicy {
			t.Fatalf("case %s: expect %v, get %v", c.name, c.expectedPolicy, *modifiedGroup.Spec.DeregisterPolicy)
		}
	}
}

func TestAdmitter_ValidateDriverCreate(t *testing.T) {
	type testCase struct {
		name        string
		driver      *lbcfapi.LoadBalancerDriver
		expectAllow bool
	}
	cases := []testCase{
		{
			name: "webhooks-not-configured",
			driver: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: "default",
				},
				Spec: lbcfapi.LoadBalancerDriverSpec{
					DriverType: string(lbcfapi.WebhookDriver),
					URL:        "http://1.1.1.1:80",
				},
			},
			expectAllow: false,
		},
		{
			name: "webhooks-partially-configured",
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
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 20 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: false,
		},
		{
			name: "webhooks-timeout-not-set",
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
						},
						{
							Name: webhooks.CreateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: false,
		},

		{
			name: "webhooks-timeout-too-long",
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
								Duration: 2 * time.Minute,
							},
						},
						{
							Name: webhooks.CreateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: false,
		},
		{
			name: "invalid-name",
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
			expectAllow: false,
		},
		{
			name: "unsupported-webhook-name",
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
							Name: "a-not-supported-webhook",
							Timeout: lbcfapi.Duration{
								Duration: 10 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: false,
		},
		{
			name: "valid",
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
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: true,
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})

	for _, c := range cases {
		raw, _ := json.Marshal(c.driver)
		ar := &v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: raw,
				},
			},
		}
		if resp := a.ValidateDriverCreate(ar); resp.Allowed != c.expectAllow {
			t.Errorf("case %s, expect %v, get %v", c.name, c.expectAllow, resp.Allowed)
		}
	}
}

func TestAdmitter_ValidateDriverDelete(t *testing.T) {
	a := fakeAdmitter(
		&notfoundLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						lbcfapi.DriverDrainingLabel: "true",
					},
				},
			},
		},
		nil,
		&notfoundBackendLister{}, &fakeSuccInvoker{})
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{},
	}
	resp := a.ValidateDriverDelete(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow, msg: %v", resp.Result.Message)
	}
}

func TestAdmitter_ValidateDriverDelete_NotDraining(t *testing.T) {
	a := fakeAdmitter(
		&notfoundLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
		},
		nil,
		&notfoundBackendLister{}, &fakeSuccInvoker{})
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{},
	}
	resp := a.ValidateDriverDelete(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateDriverDelete_LBRemaining(t *testing.T) {
	ts := metav1.Now()
	a := fakeAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &ts,
				},
			},
			list: []*lbcfapi.LoadBalancer{
				{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &ts,
					},
				},
			},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						lbcfapi.DriverDrainingLabel: "true",
					},
				},
			},
		},
		nil,
		&notfoundBackendLister{}, &fakeSuccInvoker{})
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{},
	}
	resp := a.ValidateDriverDelete(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateDriverDelete_BackendRemaining(t *testing.T) {
	ts := metav1.Now()
	a := fakeAdmitter(
		&notfoundLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						lbcfapi.DriverDrainingLabel: "true",
					},
				},
			},
		},
		nil,
		&alwaysSuccBackendLister{
			list: []*lbcfapi.BackendRecord{
				{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &ts,
					},
				},
			},
		},
		&fakeSuccInvoker{})
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{},
	}
	resp := a.ValidateDriverDelete(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateDriverUpdate(t *testing.T) {
	type testCase struct {
		name        string
		old         *lbcfapi.LoadBalancerDriver
		cur         *lbcfapi.LoadBalancerDriver
		expectAllow bool
	}
	cases := []testCase{
		{
			name: "allow-modify-timeout",
			old: &lbcfapi.LoadBalancerDriver{
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
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
					},
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
								Duration: 30 * time.Second,
							},
						},
						{
							Name: webhooks.CreateLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 30 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 30 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 30 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: true,
		},
		{
			name: "not-allow-delete-webhook",
			old: &lbcfapi.LoadBalancerDriver{
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
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeleteLoadBalancer,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.ValidateBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.GenerateBackendAddr,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.EnsureBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
						{
							Name: webhooks.DeregBackend,
							Timeout: lbcfapi.Duration{
								Duration: 15 * time.Second,
							},
						},
					},
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
								Duration: 30 * time.Second,
							},
						},
					},
				},
			},
			expectAllow: false,
		},
	}

	a := fakeAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	for _, c := range cases {
		oldRaw, _ := json.Marshal(c.old)
		curRaw, _ := json.Marshal(c.cur)
		ar := &v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: curRaw,
				},
				OldObject: runtime.RawExtension{
					Raw: oldRaw,
				},
			},
		}
		resp := a.ValidateDriverUpdate(ar)
		if resp.Allowed != c.expectAllow {
			t.Fatalf("case %s, expect %v, get %v", c.name, c.expectAllow, resp.Allowed)
		}
	}
}

func TestAdmitter_ValidateDriverUpdate_UpdatedForbiddenField(t *testing.T) {
	old := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			URL:        "http://1.1.1.1:80",
		},
	}
	cur := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			URL:        "http://another.url.com:80",
		},
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateDriverUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateDriverUpdate_CurObjInvalid(t *testing.T) {
	old := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			URL:        "http://1.1.1.1:80",
		},
	}
	cur := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			URL:        "invalid url",
		},
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateDriverUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerCreate_DriverNotExist(t *testing.T) {
	a := fakeAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	lb := &lbcfapi.LoadBalancer{
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
	}
	raw, _ := json.Marshal(lb)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	resp := a.ValidateLoadBalancerCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerCreate_DriverDraining(t *testing.T) {
	a := fakeAdmitter(&alwaysSuccLBLister{}, drainingDriverLister(), nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	lb := &lbcfapi.LoadBalancer{
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
	}
	raw, _ := json.Marshal(lb)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	resp := a.ValidateLoadBalancerCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerCreate_DriverDeleting(t *testing.T) {
	a := fakeAdmitter(&alwaysSuccLBLister{}, deletingDriverLister(), nil, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	lb := &lbcfapi.LoadBalancer{
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
	}
	raw, _ := json.Marshal(lb)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	resp := a.ValidateLoadBalancerCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerCreate_WebhookFail(t *testing.T) {
	driverLister := &alwaysSuccDriverLister{
		get: &lbcfapi.LoadBalancerDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-driver",
			},
			Spec: lbcfapi.LoadBalancerDriverSpec{
				DriverType: string(lbcfapi.WebhookDriver),
				URL:        "http://localhost:23456",
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{}, driverLister, nil, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	lb := &lbcfapi.LoadBalancer{
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
	}
	raw, _ := json.Marshal(lb)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	resp := a.ValidateLoadBalancerCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerUpdate(t *testing.T) {
	old := &lbcfapi.LoadBalancer{
		Spec: lbcfapi.LoadBalancerSpec{
			LBDriver: "test-driver",
			LBSpec: map[string]string{
				"k1": "v1",
			},
			Attributes: map[string]string{
				"a1": "v1",
			},
		},
	}
	cur := &lbcfapi.LoadBalancer{
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
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateLoadBalancerUpdate(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

func TestAdmitter_ValidateLoadBalancerUpdate_UpdatedForbiddenField(t *testing.T) {
	old := &lbcfapi.LoadBalancer{
		Spec: lbcfapi.LoadBalancerSpec{
			LBDriver: "test-driver",
			LBSpec: map[string]string{
				"k1": "v1",
			},
			Attributes: map[string]string{
				"a1": "v1",
			},
		},
	}
	cur := &lbcfapi.LoadBalancer{
		Spec: lbcfapi.LoadBalancerSpec{
			LBDriver: "test-driver",
			LBSpec: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
			Attributes: map[string]string{
				"a1": "v1",
			},
		},
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateLoadBalancerUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerUpdate_CurObjInvalid(t *testing.T) {
	old := &lbcfapi.LoadBalancer{
		Spec: lbcfapi.LoadBalancerSpec{
			LBDriver: "test-driver",
			LBSpec: map[string]string{
				"k1": "v1",
			},
			Attributes: map[string]string{
				"a1": "v1",
			},
		},
	}
	cur := &lbcfapi.LoadBalancer{
		Spec: lbcfapi.LoadBalancerSpec{
			LBDriver: "test-driver",
			LBSpec: map[string]string{
				"k1": "v1",
			},
			Attributes: map[string]string{
				"a2": "v2",
			},
			EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyIfNotSucc,
				MinPeriod: &lbcfapi.Duration{
					Duration: 1 * time.Minute,
				},
			},
		},
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateLoadBalancerUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerDelete(t *testing.T) {
	a := fakeAdmitter(&notfoundLBLister{}, &notfoundDriverLister{}, nil, &notfoundBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateLoadBalancerDelete(&v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Name:      "name",
			Namespace: "namespace",
			UID:       "12345",
		},
	})
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-lb"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     80,
						Protocol: "TCP",
					},
					{
						Port:     90,
						Protocol: "UDP",
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
	}
	raw, _ := json.Marshal(group)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	a := fakeAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lb",
					Namespace: group.Namespace,
				},
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
				},
			},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: group.Namespace,
				},
			},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_InvalidGroup(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-lb"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     0,
						Protocol: "TCP",
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
	}
	raw, _ := json.Marshal(group)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	a := fakeAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_LBNotFound(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-lb"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     80,
						Protocol: "TCP",
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
	}
	raw, _ := json.Marshal(group)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	a := fakeAdmitter(&notfoundLBLister{}, &alwaysSuccDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_LBDeleting(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-lb"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     80,
						Protocol: "TCP",
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
	}
	raw, _ := json.Marshal(group)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	ts := metav1.Now()
	a := fakeAdmitter(
		&alwaysSuccLBLister{get: &lbcfapi.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &ts,
			},
		}},
		&alwaysSuccDriverLister{}, nil, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_WebHookFail(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-lb"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     80,
						Protocol: "TCP",
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
	}
	raw, _ := json.Marshal(group)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	a := fakeAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lb",
					Namespace: group.Namespace,
				},
				Spec: lbcfapi.LoadBalancerSpec{
					LBDriver: "test-driver",
				},
			},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-driver",
					Namespace: group.Namespace,
				},
			},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupUpdate(t *testing.T) {
	old := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-loadbalancer"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     80,
						Protocol: "TCP",
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
	}
	cur := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-loadbalancer"},
			Pods: &lbcfapi.PodBackend{
				Ports: []lbcfapi.PortSelector{
					{
						Port:     8080,
						Protocol: "TCP",
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
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupUpdate(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

func TestAdmitter_ValidateBackendGroupUpdate_UpdatedForbiddenField(t *testing.T) {
	old := &lbcfapi.BackendGroup{
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
	}
	cur := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LoadBalancers: []string{"test-loadbalancer"},
			Static: []string{
				"pod-0",
			},
			Parameters: map[string]string{
				"p1": "v1",
			},
		},
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupUpdate_CurObjInvalid(t *testing.T) {
	old := &lbcfapi.BackendGroup{
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
	}
	cur := &lbcfapi.BackendGroup{
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
			EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyIfNotSucc,
				MinPeriod: &lbcfapi.Duration{
					Duration: 10 * time.Second,
				},
			},
		},
	}
	oldRaw, _ := json.Marshal(old)
	curRaw, _ := json.Marshal(cur)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: curRaw,
			},
			OldObject: runtime.RawExtension{
				Raw: oldRaw,
			},
		},
	}
	a := fakeAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		nil,
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupDelete(t *testing.T) {
	a := fakeAdmitter(&notfoundLBLister{}, &notfoundDriverLister{}, &alwaysSuccBackendGroupLister{get: &lbcfapi.BackendGroup{}}, &notfoundBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupDelete(&v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Name:      "name",
			Namespace: "namespace",
			UID:       "12345",
		},
	})
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

type alwaysSuccLBLister struct {
	get  *lbcfapi.LoadBalancer
	list []*lbcfapi.LoadBalancer
}

func (l *alwaysSuccLBLister) Get(name string) (*lbcfapi.LoadBalancer, error) {
	return l.get, nil
}

func (l *alwaysSuccLBLister) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancer, err error) {
	return l.list, nil
}

func (l *alwaysSuccLBLister) LoadBalancers(namespace string) lbcflister.LoadBalancerNamespaceLister {
	return l
}

type alwaysSuccDriverLister struct {
	get  *lbcfapi.LoadBalancerDriver
	list []*lbcfapi.LoadBalancerDriver
}

func (l *alwaysSuccDriverLister) Get(name string) (*lbcfapi.LoadBalancerDriver, error) {
	return l.get, nil
}

func (l *alwaysSuccDriverLister) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancerDriver, err error) {
	return l.list, nil
}

func (l *alwaysSuccDriverLister) LoadBalancerDrivers(namespace string) lbcflister.LoadBalancerDriverNamespaceLister {
	return l
}

type alwaysSuccBackendLister struct {
	get  *lbcfapi.BackendRecord
	list []*lbcfapi.BackendRecord
}

func (l *alwaysSuccBackendLister) Get(name string) (*lbcfapi.BackendRecord, error) {
	return l.get, nil
}

func (l *alwaysSuccBackendLister) List(selector labels.Selector) (ret []*lbcfapi.BackendRecord, err error) {
	return l.list, nil
}

func (l *alwaysSuccBackendLister) BackendRecords(namespace string) lbcflister.BackendRecordNamespaceLister {
	return l
}

type alwaysSuccBackendGroupLister struct {
	get  *lbcfapi.BackendGroup
	list []*lbcfapi.BackendGroup
}

func (l *alwaysSuccBackendGroupLister) Get(name string) (*lbcfapi.BackendGroup, error) {
	return l.get, nil
}

func (l *alwaysSuccBackendGroupLister) List(selector labels.Selector) (ret []*lbcfapi.BackendGroup, err error) {
	return l.list, nil
}

func (l *alwaysSuccBackendGroupLister) BackendGroups(namespace string) lbcflister.BackendGroupNamespaceLister {
	return l
}

type notfoundDriverLister struct {
}

func (l *notfoundDriverLister) Get(name string) (*lbcfapi.LoadBalancerDriver, error) {
	return nil, errors.NewNotFound(schema.GroupResource{}, name)
}

func (l *notfoundDriverLister) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancerDriver, err error) {
	return nil, nil
}

func (l *notfoundDriverLister) LoadBalancerDrivers(namespace string) lbcflister.LoadBalancerDriverNamespaceLister {
	return l
}

type notfoundLBLister struct{}

func (l *notfoundLBLister) Get(name string) (*lbcfapi.LoadBalancer, error) {
	return nil, errors.NewNotFound(schema.GroupResource{}, name)
}

func (l *notfoundLBLister) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancer, err error) {
	return nil, nil
}

func (l *notfoundLBLister) LoadBalancers(namespace string) lbcflister.LoadBalancerNamespaceLister {
	return l
}

type notfoundBackendLister struct{}

func (l *notfoundBackendLister) Get(name string) (*lbcfapi.BackendRecord, error) {
	return nil, errors.NewNotFound(schema.GroupResource{}, name)
}

func (l *notfoundBackendLister) List(selector labels.Selector) (ret []*lbcfapi.BackendRecord, err error) {
	return nil, nil
}

func (l *notfoundBackendLister) BackendRecords(namespace string) lbcflister.BackendRecordNamespaceLister {
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

func (c *fakeSuccInvoker) CallJudgePodDeregister(driver *lbcfapi.LoadBalancerDriver, req *webhooks.JudgePodDeregisterRequest) (*webhooks.JudgePodDeregisterResponse, error) {
	return &webhooks.JudgePodDeregisterResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Succ: true,
			Msg:  "fake succ",
		}}, nil
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

func (c *fakeFailInvoker) CallJudgePodDeregister(driver *lbcfapi.LoadBalancerDriver, req *webhooks.JudgePodDeregisterRequest) (*webhooks.JudgePodDeregisterResponse, error) {
	return &webhooks.JudgePodDeregisterResponse{
		ResponseForNoRetryHooks: webhooks.ResponseForNoRetryHooks{
			Msg: "fake fail",
		}}, nil
}

func drainingDriverLister() lbcflister.LoadBalancerDriverLister {
	return &alwaysSuccDriverLister{
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
	return &alwaysSuccDriverLister{
		get: &lbcfapi.LoadBalancerDriver{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-driver",
				DeletionTimestamp: &ts,
			},
		},
	}
}

func fakeAdmitter(
	lbLister lbcflister.LoadBalancerLister,
	driverLister lbcflister.LoadBalancerDriverLister,
	bgLister lbcflister.BackendGroupLister,
	beLister lbcflister.BackendRecordLister,
	invoker util.WebhookInvoker) Webhook {
	return &Admitter{
		lbLister:       lbLister,
		driverLister:   driverLister,
		bgLister:       bgLister,
		backendLister:  beLister,
		webhookInvoker: invoker,
	}
}

func toStringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
