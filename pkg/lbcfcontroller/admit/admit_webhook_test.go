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

package admit

import (
	"encoding/json"
	"testing"
	"time"

	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcflister "git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	"github.com/evanphx/json-patch"
	"k8s.io/api/admission/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/listers/core/v1"
)

func TestAdmitter_MutateLB(t *testing.T) {
	a := NewAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})

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
	if rsp.Allowed != true {
		t.Fatalf("expect allow")
	} else if string(rsp.Patch) != `[{"op":"add","path":"/metadata/finalizers","value":["lbcf.tke.cloud.tencent.com/delete-load-loadbalancer"]}]` {
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
	if rsp.Allowed != true {
		t.Fatalf("expect allow")
	} else if string(rsp.Patch) != `[{"op":"add","path":"/metadata/finalizers/-","value":"lbcf.tke.cloud.tencent.com/delete-load-loadbalancer"}]` {
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
	a := NewAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	rsp := a.MutateDriver(&v1beta1.AdmissionReview{})
	if !rsp.Allowed {
		t.Fatalf("expect always allow")
	}
}

func TestAdmitter_MutateBackendGroup(t *testing.T) {
	a := NewAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	group := &lbcfapi.BackendGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backendgroup",
			Namespace: "test",
		},
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
				},
				ByLabel: &lbcfapi.SelectPodByLabel{
					Selector: map[string]string{
						"key1": "value1",
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(group)
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
		t.Fatalf(err.Error())
	}
	modifiedGroup := &lbcfapi.BackendGroup{}
	if err := json.Unmarshal(modified, modifiedGroup); err != nil {
		t.Fatalf(err.Error())
	}

	if modifiedGroup.Labels[lbcfapi.LabelLBName] != group.Spec.LBName {
		t.Fatalf("expect label value %s, get %s", group.Spec.LBName, modifiedGroup.Labels[lbcfapi.LabelLBName])
	} else if modifiedGroup.Spec.Pods.Port.Protocol != "TCP" {
		t.Fatalf("get protocol %s", modifiedGroup.Spec.Pods.Port.Protocol)
	}

	extraKey := "key1"
	extraValue := "v1"
	group = &lbcfapi.BackendGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backendgroup",
			Namespace: "test",
			Labels: map[string]string{
				extraKey:            extraValue,
				lbcfapi.LabelLBName: "a-random-value",
			},
		},
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
				},
				ByLabel: &lbcfapi.SelectPodByLabel{
					Selector: map[string]string{
						"key1": "value1",
					},
				},
			},
		},
	}
	raw, _ = json.Marshal(group)
	rsp = a.MutateBackendGroup(&v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	})
	if !rsp.Allowed {
		t.Fatalf("expect always allow")
	}
	patch, err = jsonpatch.DecodePatch(rsp.Patch)
	if err != nil {
		t.Fatalf(err.Error())
	}
	modified, err = patch.Apply(raw)
	if err != nil {
		t.Fatalf(err.Error())
	}
	modifiedGroup = &lbcfapi.BackendGroup{}
	if err := json.Unmarshal(modified, modifiedGroup); err != nil {
		t.Fatalf(err.Error())
	}
	if modifiedGroup.Labels[extraKey] != extraValue {
		t.Fatalf("key value lost")
	} else if modifiedGroup.Labels[lbcfapi.LabelLBName] != group.Spec.LBName {
		t.Fatalf("expect label value %s, get %s", group.Spec.LBName, modifiedGroup.Labels[lbcfapi.LabelLBName])
	} else if modifiedGroup.Spec.Pods.Port.Protocol != "TCP" {
		t.Fatalf("get protocol %s", modifiedGroup.Spec.Pods.Port.Protocol)
	}
}

func TestAdmitter_ValidateDriverCreate(t *testing.T) {
	a := NewAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})

	// case 1: valid driver, expect allow
	driver := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        "http://1.1.1.1:80",
		},
	}
	raw, _ := json.Marshal(driver)
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	if resp := a.ValidateDriverCreate(ar); !resp.Allowed {
		t.Fatalf("expect allow, msg: %v", resp.Result.Message)
	}

	// case 2: invalid driver, expect not allow
	driver = &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-name",
			Namespace: "kube-system",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        "http://1.1.1.1:80",
		},
	}
	raw, _ = json.Marshal(driver)
	ar = &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	if resp := a.ValidateDriverCreate(ar); resp.Allowed {
		t.Fatalf("expect not allow")
	}

	// case 3: loadbalancer decode error, expect not allow
	ar = &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Raw: []byte("invalid json"),
			},
		},
	}
	if resp := a.ValidateDriverCreate(ar); resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateDriverDelete(t *testing.T) {
	a := NewAdmitter(
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
	a := NewAdmitter(
		&notfoundLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
		},
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
	a := NewAdmitter(
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
	a := NewAdmitter(
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
	old := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        "http://1.1.1.1:80",
		},
	}
	cur := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        "http://1.1.1.1:80",
			Webhooks: []lbcfapi.WebhookConfig{
				{
					Name: webhooks.ValidateLoadBalancer,
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
	a := NewAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateDriverUpdate(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow")
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
			Url:        "http://1.1.1.1:80",
		},
	}
	cur := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        "http://another.url.com:80",
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
	a := NewAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
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
			Url:        "http://1.1.1.1:80",
		},
	}
	cur := &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-driver",
			Namespace: "default",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        "invalid url",
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
	a := NewAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateDriverUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerCreate_DriverNotExist(t *testing.T) {
	a := NewAdmitter(&alwaysSuccLBLister{}, &notfoundDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
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
					30 * time.Second,
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
	a := NewAdmitter(&alwaysSuccLBLister{}, drainingDriverLister(), &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
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
					30 * time.Second,
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
	a := NewAdmitter(&alwaysSuccLBLister{}, deletingDriverLister(), &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
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
					30 * time.Second,
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
				Url:        "http://localhost:23456",
			},
		},
	}
	a := NewAdmitter(&alwaysSuccLBLister{}, driverLister, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
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
					30 * time.Second,
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
					1 * time.Minute,
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
	a := NewAdmitter(&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
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
	a := NewAdmitter(&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
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
					1 * time.Minute,
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
	a := NewAdmitter(&alwaysSuccLBLister{},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateLoadBalancerUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateLoadBalancerDelete(t *testing.T) {
	a := NewAdmitter(&notfoundLBLister{}, &notfoundDriverLister{}, &notfoundBackendLister{}, &fakeSuccInvoker{})
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
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
					Protocol:   "TCP",
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
					30 * time.Second,
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
	a := NewAdmitter(&alwaysSuccLBLister{
		get: &lbcfapi.LoadBalancer{},
	}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_InvalidGroup(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 0,
					Protocol:   "TCP",
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
					30 * time.Second,
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
	a := NewAdmitter(&alwaysSuccLBLister{}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_LBNotFound(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
					Protocol:   "TCP",
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
					30 * time.Second,
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
	a := NewAdmitter(&notfoundLBLister{}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_LBDeleting(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
					Protocol:   "TCP",
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
					30 * time.Second,
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
	a := NewAdmitter(
		&alwaysSuccLBLister{get: &lbcfapi.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &ts,
			},
		}},
		&alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupCreate_WebHookFail(t *testing.T) {
	group := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-lb",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
					Protocol:   "TCP",
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
					30 * time.Second,
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
	a := NewAdmitter(&alwaysSuccLBLister{
		get: &lbcfapi.LoadBalancer{},
	}, &alwaysSuccDriverLister{}, &alwaysSuccBackendLister{}, &fakeFailInvoker{})
	resp := a.ValidateBackendGroupCreate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupUpdate(t *testing.T) {
	old := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-loadbalancer",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
					Protocol:   "TCP",
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
			LBName: "test-loadbalancer",
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 8080,
					Protocol:   "TCP",
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
	a := NewAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupUpdate(ar)
	if !resp.Allowed {
		t.Fatalf("expect allow")
	}
}

func TestAdmitter_ValidateBackendGroupUpdate_UpdatedForbiddenField(t *testing.T) {
	old := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-loadbalancer",
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
			Parameters: map[string]string{
				"p1": "v1",
			},
		},
	}
	cur := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-loadbalancer",
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
	a := NewAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupUpdate_CurObjInvalid(t *testing.T) {
	old := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-loadbalancer",
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
			Parameters: map[string]string{
				"p1": "v1",
			},
		},
	}
	cur := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			LBName: "test-loadbalancer",
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
			Parameters: map[string]string{
				"p1": "v1",
			},
			EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
				Policy: lbcfapi.PolicyIfNotSucc,
				MinPeriod: &lbcfapi.Duration{
					10 * time.Second,
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
	a := NewAdmitter(
		&alwaysSuccLBLister{
			get: &lbcfapi.LoadBalancer{},
		},
		&alwaysSuccDriverLister{
			get: &lbcfapi.LoadBalancerDriver{},
		},
		&alwaysSuccBackendLister{}, &fakeSuccInvoker{})
	resp := a.ValidateBackendGroupUpdate(ar)
	if resp.Allowed {
		t.Fatalf("expect not allow")
	}
}

func TestAdmitter_ValidateBackendGroupDelete(t *testing.T) {
	a := NewAdmitter(&notfoundLBLister{}, &notfoundDriverLister{}, &notfoundBackendLister{}, &fakeSuccInvoker{})
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

type alwaysSuccPodLister struct {
	getPod   *apiv1.Pod
	listPods []*apiv1.Pod
}

func (l *alwaysSuccPodLister) Get(name string) (*apiv1.Pod, error) {
	return l.getPod, nil
}

func (l *alwaysSuccPodLister) List(selector labels.Selector) (ret []*apiv1.Pod, err error) {
	return l.listPods, nil
}

func (l *alwaysSuccPodLister) Pods(namespace string) v1.PodNamespaceLister {
	return l
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
