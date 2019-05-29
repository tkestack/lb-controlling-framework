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
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/controller"
	"testing"

	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
)

func TestDriverControllerSyncDriverCreate(t *testing.T) {
	driver := newFakeDriver("kube-system", fmt.Sprintf("%s%s", lbcfapi.SystemDriverPrefix, "driver"))

	key, _ := controller.KeyFunc(driver)
	ctrl := newDriverController(fake.NewSimpleClientset(driver), &fakeDriverLister{
		get: driver,
	})
	result := ctrl.syncDriver(key)
	if !result.IsSucc() {
		t.Logf("%v", result.GetError())
		t.Fatalf("expect succ result, get %#v", result)
	}
}

func TestDriverControllerSyncDriverAccepted(t *testing.T) {
	driver := newFakeDriver("kube-system", fmt.Sprintf("%s%s", lbcfapi.SystemDriverPrefix, "driver"))
	driver.Status = lbcfapi.LoadBalancerDriverStatus{
		Conditions: []lbcfapi.LoadBalancerDriverCondition{
			{
				Type:   lbcfapi.DriverAccepted,
				Status: lbcfapi.ConditionTrue,
			},
		},
	}
	key, _ := controller.KeyFunc(driver)
	ctrl := newDriverController(fake.NewSimpleClientset(), &fakeDriverLister{
		get: driver,
	})
	result := ctrl.syncDriver(key)
	if !result.IsSucc() {
		t.Logf("%v", result.GetError())
		t.Fatalf("expect succ result, get %#v", result)
	}
}

func TestDriverControllerSyncDriverNotFound(t *testing.T) {
	driver := newFakeDriver("kube-system", fmt.Sprintf("%s%s", lbcfapi.SystemDriverPrefix, "driver"))

	key, _ := controller.KeyFunc(driver)
	ctrl := newDriverController(fake.NewSimpleClientset(), &fakeDriverLister{})
	result := ctrl.syncDriver(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %v", result)
	}
}

func TestDriverControllerSyncDriverDeleting(t *testing.T) {
	driver := newFakeDriver("kube-system", fmt.Sprintf("%s%s", lbcfapi.SystemDriverPrefix, "driver"))
	ts := metav1.Now()
	driver.DeletionTimestamp = &ts

	key, _ := controller.KeyFunc(driver)
	ctrl := newDriverController(fake.NewSimpleClientset(), &fakeDriverLister{
		get: driver,
	})
	result := ctrl.syncDriver(key)
	if !result.IsSucc() {
		t.Fatalf("expect succ result, get %v", result)
	}
}
