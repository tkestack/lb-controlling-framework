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
	"encoding/json"
	"fmt"

	lbcf "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	admission "k8s.io/api/admission/v1beta1"
)

type AdmitWebhook interface {
	ValidateAdmitWebhook
	MutateAdmitWebhook
}

type ValidateAdmitWebhook interface {
	ValidateLoadBalancerCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateLoadBalancerUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateLoadBalancerDelete(*admission.AdmissionReview) *admission.AdmissionResponse

	ValidateDriverCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateDriverUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateDriverDelete(*admission.AdmissionReview) *admission.AdmissionResponse

	ValidateBackendGroupCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateBackendGroupUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateBackendGroupDelete(*admission.AdmissionReview) *admission.AdmissionResponse
}

type MutateAdmitWebhook interface {
	MutateLB(*admission.AdmissionReview) *admission.AdmissionResponse
	MutateDriver(*admission.AdmissionReview) *admission.AdmissionResponse
	MutateBackendGroup(*admission.AdmissionReview) *admission.AdmissionResponse
}

const (
	lbFinalizerPatch string = `[
		 {"op":"add","path":"/metadata/finalizers","value":["lbcf.tke.cloud.tencent.com/delete-load-loadbalancer"]}
	]`
	bgFinalizerPatch string = `[
		 {"op":"add","path":"/metadata/finalizers","value":["lbcf.tke.cloud.tencent.com/deregister-backend-group"]}
	]`
)

func (c *Controller) MutateLB(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = []byte(lbFinalizerPatch)
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

func (c *Controller) MutateDriver(ar *admission.AdmissionReview) (adResponse *admission.AdmissionResponse) {
	return toAdmissionResponse(nil)
}

func (c *Controller) MutateBackendGroup(*admission.AdmissionReview) *admission.AdmissionResponse {
	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = []byte(bgFinalizerPatch)
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

func (c *Controller) ValidateLoadBalancerCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	lb := lbcf.LoadBalancer{}
	if err := json.Unmarshal(ar.Request.Object.Raw, lb); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}

	driverNamespace := getDriverNamespace(lb.Spec.LBDriver, lb.Namespace)
	driver, exist := c.driverCtrl.getDriver(driverNamespace, lb.Spec.LBDriver)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", lb.Spec.LBDriver, driverNamespace))
	}
	if driver.IsDraining() {
		return toAdmissionResponse(fmt.Errorf("driver %q is draining, all LoadBalancer creating operation for that dirver is denied", lb.Spec.LBDriver))
	}

	req := &ValidateLoadBalancerRequest{
		LBSpec:     lb.Spec.LBSpec,
		Operation:  OperationCreate,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := driver.CallValidateLoadBalancer(req)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("call webhook error, webhook: validateLoadBalancer, err: %v", err))
	} else if !rsp.Succ {
		return toAdmissionResponse(fmt.Errorf("invalid LoadBalancer: %s", rsp.Msg))
	}

	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateLoadBalancerUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &lbcf.LoadBalancer{}
	oldObj := &lbcf.LoadBalancer{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}
	if !LBUpdatedFieldsAllowed(curObj, oldObj) {
		return toAdmissionResponse(fmt.Errorf("update to non-attributes fields is not permitted"))
	}

	driverNamespace := getDriverNamespace(curObj.Spec.LBDriver, curObj.Namespace)
	driver, exist := c.driverCtrl.getDriver(driverNamespace, curObj.Spec.LBDriver)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", curObj.Spec.LBDriver, driverNamespace))
	}

	needUpdate := false
	if len(curObj.Spec.Attributes) != len(oldObj.Spec.Attributes) {
		needUpdate = true
	} else {
		for k, v := range curObj.Spec.Attributes {
			if oldObj.Spec.Attributes[k] == v {
				continue
			}
			needUpdate = true
			break
		}
	}

	if !needUpdate {
		return toAdmissionResponse(nil)
	}

	req := &ValidateLoadBalancerRequest{
		LBSpec:        curObj.Spec.LBSpec,
		Operation:     OperationUpdate,
		Attributes:    curObj.Spec.Attributes,
		OldAttributes: oldObj.Spec.Attributes,
	}
	rsp, err := driver.CallValidateLoadBalancer(req)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("call webhook error, webhook: validateLoadBalancer, err: %v", err))
	} else if !rsp.Succ {
		return toAdmissionResponse(fmt.Errorf("invalid LoadBalancer: %s", rsp.Msg))
	}

	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateLoadBalancerDelete(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateDriverCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	d := &lbcf.LoadBalancerDriver{}
	if err := json.Unmarshal(ar.Request.Object.Raw, d); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}

	errList := ValidateLoadBalancerDriver(d)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateDriverUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &lbcf.LoadBalancerDriver{}
	oldObj := &lbcf.LoadBalancerDriver{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}

	if !DriverUpdatedFieldsAllowed(curObj, oldObj) {
		return toAdmissionResponse(fmt.Errorf("update to LoadBalancerUpdate is not permitted"))
	}
	errList := ValidateLoadBalancerDriver(curObj)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}
	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateDriverDelete(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	driver := &lbcf.LoadBalancerDriver{}
	if err := json.Unmarshal(ar.Request.Object.Raw, driver); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}
	if isDriverDraining(driver.Labels) {
		return toAdmissionResponse(fmt.Errorf("LoadBalancerDriver must be label with %s:\"true\" before delete", driverDrainingLabel))
	}

	lbList, err := c.lbCtrl.listLoadBalancerByDriver(driver.Name, driver.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("unable to list LoadBalancers for driver, err: %v", err))
	} else if len(lbList) > 0 {
		return toAdmissionResponse(fmt.Errorf("all LoadBalancers must be deleted, %d remaining", len(lbList)))
	}

	beList, err := c.backendCtrl.listBackendByDriver(driver.Name, driver.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("unable to list BackendRecords for driver, err: %v", err))
	} else if len(beList) > 0 {
		return toAdmissionResponse(fmt.Errorf("all BackendRecord must be deregistered, %d remaining", len(beList)))
	}
	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateBackendGroupCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	bg := &lbcf.BackendGroup{}
	if err := json.Unmarshal(ar.Request.Object.Raw, bg); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode BackendGroup failed: %v", err))
	}
	errList := ValidateBackendGroup(bg)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	lb, err := c.lbCtrl.getLoadBalancer(bg.Spec.LBName, bg.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("loadbalancer not found, LoadBalancer must be created before BackendGroup"))
	}
	if lb.DeletionTimestamp != nil {
		return toAdmissionResponse(fmt.Errorf("operation denied: loadbalancer %q is deleting", lb.Name))
	}
	driverNamespace := getDriverNamespace(lb.Spec.LBDriver, bg.Namespace)
	driver, exist := c.driverCtrl.getDriver(lb.Spec.LBDriver, driverNamespace)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", lb.Spec.LBDriver, driverNamespace))
	}
	req := &ValidateBackendRequest{
		BackendType: string(getBackendType(bg)),
		LBInfo:      lb.Status.LBInfo,
		Operation:   OperationCreate,
		Parameters:  bg.Spec.Parameters,
	}
	rsp, err := driver.CallValidateBackend(req)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("call webhook error, webhook validateBackend, err: %v", err))
	} else if !rsp.Succ {
		return toAdmissionResponse(fmt.Errorf("invalid Backend, err: %v", err))
	}

	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateBackendGroupUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &lbcf.BackendGroup{}
	oldObj := &lbcf.BackendGroup{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}

	if !BackendGroupUpdateFieldsAllowed(curObj, oldObj) {
		return toAdmissionResponse(fmt.Errorf("update to backend type is not permitted"))
	}

	needUpdate := false
	if len(curObj.Spec.Parameters) != len(oldObj.Spec.Parameters) {
		needUpdate = true
	} else {
		for k, v := range curObj.Spec.Parameters {
			if oldObj.Spec.Parameters[k] == v {
				continue
			}
			needUpdate = true
			break
		}
	}
	if !needUpdate {
		return toAdmissionResponse(nil)
	}

	lb, err := c.lbCtrl.getLoadBalancer(curObj.Spec.LBName, curObj.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("loadbalancer not found, LoadBalancer must be created before BackendGroup"))
	}
	driverNamespace := getDriverNamespace(lb.Spec.LBDriver, curObj.Namespace)
	driver, exist := c.driverCtrl.getDriver(lb.Spec.LBDriver, driverNamespace)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", lb.Spec.LBDriver, driverNamespace))
	}

	req := &ValidateBackendRequest{
		BackendType: string(getBackendType(curObj)),
		LBInfo:      lb.Status.LBInfo,
		Operation:   OperationUpdate,
		Parameters:  curObj.Spec.Parameters,
	}
	rsp, err := driver.CallValidateBackend(req)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("call webhook error, webhook validateBackend, err: %v", err))
	} else if !rsp.Succ {
		return toAdmissionResponse(fmt.Errorf("invalid Backend, err: %v", err))
	}
	return toAdmissionResponse(nil)
}

func (c *Controller) ValidateBackendGroupDelete(*admission.AdmissionReview) *admission.AdmissionResponse {
	return toAdmissionResponse(nil)
}
