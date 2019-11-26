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
	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"github.com/evanphx/json-patch"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"testing"
)

func TestAddLabel(t *testing.T) {
	type testCase struct {
		obj         *v1.Pod
		existkey    string
		existValue  string
		newKey      string
		newValue    string
		createLabel bool
		isReplace   bool
	}
	cases := []testCase{
		{
			obj:         &v1.Pod{},
			newKey:      "test/key",
			newValue:    "test/value",
			createLabel: true,
			isReplace:   false,
		},
		{
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"another/key": "another/value",
					},
				},
			},
			existkey:    "another/key",
			existValue:  "another/value",
			newKey:      "test/key",
			newValue:    "test/value",
			createLabel: false,
			isReplace:   false,
		},
		{
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"test/key": "another/value",
					},
				},
			},
			existkey:    "test/key",
			existValue:  "another/value",
			newKey:      "test/key",
			newValue:    "test/value",
			createLabel: false,
			isReplace:   true,
		},
	}

	for _, c := range cases {
		origin, err := json.Marshal(c.obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		p, err := json.Marshal([]Patch{addLabel(c.createLabel, c.isReplace, c.newKey, c.newValue)})
		if err != nil {
			t.Fatal(err.Error())
		}
		patch, err := jsonpatch.DecodePatch(p)
		if err != nil {
			t.Fatal(err.Error())
		}
		modified, err := patch.Apply(origin)
		if err != nil {
			t.Fatal(err.Error())
		}
		modifiedObj := &v1.Pod{}
		if err := json.Unmarshal(modified, modifiedObj); err != nil {
			t.Fatal(err.Error())
		}
		v, ok := modifiedObj.Labels[c.newKey]
		if !ok {
			t.Fatalf("label not added")
		} else if v != c.newValue {
			t.Fatalf("expect value %s, get %s", c.newValue, v)
		}
		if c.existkey != "" {
			v, ok := modifiedObj.Labels[c.existkey]
			if !ok {
				t.Fatalf("existing key deleted")
			}
			if c.existkey == c.newKey && v != c.newValue {
				t.Fatalf("exist key should be replaced")
			} else if c.existkey != c.newKey && v != c.existValue {
				t.Fatalf("exist key should not be replaced")
			}
		}
	}
}

func TestAddFinalizer(t *testing.T) {
	type testCase struct {
		obj             *v1.Pod
		newFinalizer    string
		createFinalizer bool
	}

	cases := []testCase{
		{
			obj:             &v1.Pod{},
			newFinalizer:    "test/finalizer",
			createFinalizer: true,
		},
		{
			obj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{
						"another/finalizer",
					},
				},
			},
			newFinalizer:    "test/finalizer",
			createFinalizer: false,
		},
	}

	for _, c := range cases {
		origin, err := json.Marshal(c.obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		p, err := json.Marshal([]Patch{addFinalizer(c.createFinalizer, c.newFinalizer)})
		if err != nil {
			t.Fatal(err.Error())
		}
		patch, err := jsonpatch.DecodePatch(p)
		if err != nil {
			t.Fatal(err.Error())
		}
		modified, err := patch.Apply(origin)
		if err != nil {
			t.Fatal(err.Error())
		}
		modifiedObj := &v1.Pod{}
		if err := json.Unmarshal(modified, modifiedObj); err != nil {
			t.Fatal(err.Error())
		}

		if c.createFinalizer {
			if len(modifiedObj.Finalizers) != 1 {
				t.Fatalf("expect 1")
			}
			if modifiedObj.Finalizers[0] != c.newFinalizer {
				t.Fatalf("expect %s, get %s", c.newFinalizer, modifiedObj.Finalizers[0])
			}
		} else {
			if len(modifiedObj.Finalizers) != len(c.obj.Finalizers)+1 {
				t.Fatalf("expect %d, get %d", len(c.obj.Finalizers)+1, len(modifiedObj.Finalizers))
			}
			fs := sets.NewString(modifiedObj.Finalizers...)
			for _, exist := range c.obj.Finalizers {
				if !fs.Has(exist) {
					t.Fatalf("%s not found", exist)
				}
			}
			if !fs.Has(c.newFinalizer) {
				t.Fatalf("%s not added", c.newFinalizer)
			}
		}
	}
}

func TestDefaultProtocolOfPodBackend(t *testing.T) {
	podGroup := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			Pods: &lbcfapi.PodBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
				},
			},
		},
	}
	origin, err := json.Marshal(podGroup)
	if err != nil {
		t.Fatal(err.Error())
	}
	p, err := json.Marshal([]Patch{defaultPodProtocol()})
	if err != nil {
		t.Fatal(err.Error())
	}
	patch, err := jsonpatch.DecodePatch(p)
	if err != nil {
		t.Fatal(err.Error())
	}
	modified, err := patch.Apply(origin)
	if err != nil {
		t.Fatal(err.Error())
	}
	modifiedObj := &lbcfapi.BackendGroup{}
	if err := json.Unmarshal(modified, modifiedObj); err != nil {
		t.Fatal(err.Error())
	}
	ps := modifiedObj.Spec.Pods.Port
	if ps.PortNumber != podGroup.Spec.Pods.Port.PortNumber || ps.Protocol != "TCP" {
		t.Fatalf("expect %d/%s, get %d/%s", podGroup.Spec.Pods.Port.PortNumber, "TCP", ps.PortNumber, ps.Protocol)
	}
}

func TestDefaultProtocolOfSvcBackend(t *testing.T) {
	svcGroup := &lbcfapi.BackendGroup{
		Spec: lbcfapi.BackendGroupSpec{
			Service: &lbcfapi.ServiceBackend{
				Port: lbcfapi.PortSelector{
					PortNumber: 80,
				},
			},
		},
	}
	origin, err := json.Marshal(svcGroup)
	if err != nil {
		t.Fatal(err.Error())
	}
	p, err := json.Marshal([]Patch{defaultSvcProtocol()})
	if err != nil {
		t.Fatal(err.Error())
	}
	patch, err := jsonpatch.DecodePatch(p)
	if err != nil {
		t.Fatal(err.Error())
	}
	modified, err := patch.Apply(origin)
	if err != nil {
		t.Fatal(err.Error())
	}
	modifiedObj := &lbcfapi.BackendGroup{}
	if err := json.Unmarshal(modified, modifiedObj); err != nil {
		t.Fatal(err.Error())
	}
	ps := modifiedObj.Spec.Service.Port
	if ps.PortNumber != svcGroup.Spec.Service.Port.PortNumber || ps.Protocol != "TCP" {
		t.Fatalf("expect %d/%s, get %d/%s", svcGroup.Spec.Service.Port.PortNumber, "TCP", ps.PortNumber, ps.Protocol)
	}
}
