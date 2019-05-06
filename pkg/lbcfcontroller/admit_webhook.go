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
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/klog"
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

func (c *Controller) MutateLB(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	obj := &lbcf.LoadBalancer{}
	err := json.Unmarshal(ar.Request.Object.Raw, obj)
	if err != nil {
		return toAdmissionResponse(err)
	}
	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = util.MakeFinalizerPatch(len(obj.Finalizers) == 0, lbcf.FinalizerDeleteLB)
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

func (c *Controller) MutateDriver(ar *admission.AdmissionReview) (adResponse *admission.AdmissionResponse) {
	return toAdmissionResponse(nil)
}

func (c *Controller) MutateBackendGroup(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	obj := &lbcf.BackendGroup{}
	err := json.Unmarshal(ar.Request.Object.Raw, obj)
	if err != nil {
		return toAdmissionResponse(err)
	}
	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = util.MakeFinalizerPatch(len(obj.Finalizers) == 0, lbcf.FinalizerDeregisterBackendGroup)
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

func (c *Controller) ValidateLoadBalancerCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	lb := &lbcf.LoadBalancer{}
	if err := json.Unmarshal(ar.Request.Object.Raw, lb); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}

	driverNamespace := util.GetDriverNamespace(lb.Spec.LBDriver, lb.Namespace)
	driver, exist := c.driverCtrl.getDriver(driverNamespace, lb.Spec.LBDriver)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", lb.Spec.LBDriver, driverNamespace))
	}
	if driver.IsDraining() {
		return toAdmissionResponse(fmt.Errorf("driver %q is draining, all LoadBalancer creating operation for that dirver is denied", lb.Spec.LBDriver))
	}

	req := &webhooks.ValidateLoadBalancerRequest{
		LBSpec:     lb.Spec.LBSpec,
		Operation:  webhooks.OperationCreate,
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

	driverNamespace := util.GetDriverNamespace(curObj.Spec.LBDriver, curObj.Namespace)
	driver, exist := c.driverCtrl.getDriver(driverNamespace, curObj.Spec.LBDriver)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", curObj.Spec.LBDriver, driverNamespace))
	}

	if util.MapEqual(curObj.Spec.Attributes, oldObj.Spec.Attributes) {
		return toAdmissionResponse(nil)
	}

	req := &webhooks.ValidateLoadBalancerRequest{
		LBSpec:        curObj.Spec.LBSpec,
		Operation:     webhooks.OperationUpdate,
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
	klog.Infof("start ValidateDriverCreate")
	d := &lbcf.LoadBalancerDriver{}
	if err := json.Unmarshal(ar.Request.Object.Raw, d); err != nil {
		klog.Errorf(err.Error())
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
	klog.Infof("raw: %s", string(ar.Request.Object.Raw))
	if err := json.Unmarshal(ar.Request.Object.Raw, driver); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}
	if util.IsDriverDraining(driver.Labels) {
		return toAdmissionResponse(fmt.Errorf("LoadBalancerDriver must be label with %s:\"true\" before delete", lbcf.DriverDrainingLabel))
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

	lb, err := c.lbCtrl.getLoadBalancer(bg.Namespace, bg.Spec.LBName)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("loadbalancer not found, LoadBalancer must be created before BackendGroup"))
	}
	if lb.DeletionTimestamp != nil {
		return toAdmissionResponse(fmt.Errorf("operation denied: loadbalancer %q is deleting", lb.Name))
	}
	driverNamespace := util.GetDriverNamespace(lb.Spec.LBDriver, bg.Namespace)
	driver, exist := c.driverCtrl.getDriver(driverNamespace, lb.Spec.LBDriver)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", lb.Spec.LBDriver, driverNamespace))
	}
	req := &webhooks.ValidateBackendRequest{
		BackendType: string(util.GetBackendType(bg)),
		LBInfo:      lb.Status.LBInfo,
		Operation:   webhooks.OperationCreate,
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

	if util.MapEqual(curObj.Spec.Parameters, oldObj.Spec.Parameters) {
		return toAdmissionResponse(nil)
	}

	lb, err := c.lbCtrl.getLoadBalancer(curObj.Spec.LBName, curObj.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("loadbalancer not found, LoadBalancer must be created before BackendGroup"))
	}
	driverNamespace := util.GetDriverNamespace(lb.Spec.LBDriver, curObj.Namespace)
	driver, exist := c.driverCtrl.getDriver(lb.Spec.LBDriver, driverNamespace)
	if !exist {
		return toAdmissionResponse(fmt.Errorf("driver %q not found in namespace %s", lb.Spec.LBDriver, driverNamespace))
	}

	req := &webhooks.ValidateBackendRequest{
		BackendType: string(util.GetBackendType(curObj)),
		LBInfo:      lb.Status.LBInfo,
		Operation:   webhooks.OperationUpdate,
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
