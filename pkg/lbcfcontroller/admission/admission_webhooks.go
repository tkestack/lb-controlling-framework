/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */

package admission

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	v1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"

	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/context"
	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	lbcflister "tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

// Webhook is an abstract interface for testability
type Webhook interface {
	ValidatingAdmissionWebhook
	MutatingAdmissionWebhook
}

// ValidatingAdmissionWebhook is an abstract interface for testability
type ValidatingAdmissionWebhook interface {
	ValidateLoadBalancerCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateLoadBalancerUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateLoadBalancerDelete(*admission.AdmissionReview) *admission.AdmissionResponse

	ValidateDriverCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateDriverUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateDriverDelete(*admission.AdmissionReview) *admission.AdmissionResponse

	ValidateBackendGroupCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateBackendGroupUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateBackendGroupDelete(*admission.AdmissionReview) *admission.AdmissionResponse

	ValidateBindCreate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateBindUpdate(*admission.AdmissionReview) *admission.AdmissionResponse
	ValidateBindDelete(*admission.AdmissionReview) *admission.AdmissionResponse
}

// MutatingAdmissionWebhook is an abstract interface for testability
type MutatingAdmissionWebhook interface {
	MutateLB(*admission.AdmissionReview) *admission.AdmissionResponse
	MutateDriver(*admission.AdmissionReview) *admission.AdmissionResponse
	MutateBackendGroup(*admission.AdmissionReview) *admission.AdmissionResponse
}

// NewAdmitter creates a new instance of Webhook
func NewAdmitter(ctx *context.Context, invoker util.WebhookInvoker) Webhook {
	return &Admitter{
		lbLister:       ctx.LBInformer.Lister(),
		driverLister:   ctx.LBDriverInformer.Lister(),
		backendLister:  ctx.BRInformer.Lister(),
		bgLister:       ctx.BGInformer.Lister(),
		webhookInvoker: invoker,
		dryRun:         ctx.IsDryRun(),
	}
}

// Admitter is an implementation of Webhook
type Admitter struct {
	lbLister      lbcflister.LoadBalancerLister
	driverLister  lbcflister.LoadBalancerDriverLister
	bgLister      lbcflister.BackendGroupLister
	backendLister lbcflister.BackendRecordLister

	webhookInvoker util.WebhookInvoker
	dryRun         bool
}

// MutateLB implements MutatingWebHook for LoadBalancer
func (a *Admitter) MutateLB(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	obj := &lbcfapi.LoadBalancer{}
	err := json.Unmarshal(ar.Request.Object.Raw, obj)
	if err != nil {
		return toAdmissionResponse(err)
	}
	var patches []Patch
	patches = append(patches, addFinalizer(len(obj.Finalizers) == 0, lbcfapi.FinalizerDeleteLB))
	p, err := json.Marshal(patches)
	if err != nil {
		toAdmissionResponse(err)
	}

	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = p
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

// MutateDriver implements MutatingWebHook for LoadBalancerDriver
func (a *Admitter) MutateDriver(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	obj := &lbcfapi.LoadBalancerDriver{}
	err := json.Unmarshal(ar.Request.Object.Raw, obj)
	if err != nil {
		return toAdmissionResponse(err)
	}

	dPatch := &driverPatch{obj: obj}
	dPatch.setWebhook()

	p, err := json.Marshal(dPatch.patch())
	if err != nil {
		toAdmissionResponse(err)
	}

	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = p
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

// MutateBackendGroup implements MutatingWebHook for BackendGroup
func (a *Admitter) MutateBackendGroup(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	obj := &lbcfapi.BackendGroup{}
	err := json.Unmarshal(ar.Request.Object.Raw, obj)
	if err != nil {
		return toAdmissionResponse(err)
	}

	bgPatch := &backendGroupPatch{obj: obj}
	bgPatch.addLabel()
	bgPatch.convertLoadBalancers()
	bgPatch.convertPortSelector()
	bgPatch.setDefaultDeregisterPolicy()

	p, err := json.Marshal(bgPatch.patch())
	if err != nil {
		toAdmissionResponse(err)
	}

	reviewResponse := &admission.AdmissionResponse{}
	reviewResponse.Allowed = true
	reviewResponse.Patch = p
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return reviewResponse
}

// ValidateLoadBalancerCreate implements ValidatingWebHook for LoadBalancer creating
func (a *Admitter) ValidateLoadBalancerCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	lb := &lbcfapi.LoadBalancer{}
	if err := json.Unmarshal(ar.Request.Object.Raw, lb); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}

	errList := ValidateLoadBalancer(lb)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	driverNamespace := util.NamespaceOfSharedObj(lb.Spec.LBDriver, lb.Namespace)
	driver, err := a.driverLister.LoadBalancerDrivers(driverNamespace).Get(lb.Spec.LBDriver)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("retrieve driver %s/%s failed: %v", driverNamespace, lb.Spec.LBDriver, err))
	}
	if util.IsDriverDraining(driver) {
		return toAdmissionResponse(fmt.Errorf("driver %q is draining, all LoadBalancer creating operation for that dirver is denied", lb.Spec.LBDriver))
	} else if driver.DeletionTimestamp != nil {
		return toAdmissionResponse(fmt.Errorf("driver %q is deleting, all LoadBalancer creating operation for that dirver is denied", lb.Spec.LBDriver))
	}
	req := &webhooks.ValidateLoadBalancerRequest{
		LBSpec:     lb.Spec.LBSpec,
		Operation:  webhooks.OperationCreate,
		Attributes: lb.Spec.Attributes,
		DryRun:     a.dryRun,
	}
	if a.dryRun {
		klog.Infof("[dry-run] webhook: validateLoadBalancer, LoadBalancer: %s/%s",
			lb.Namespace, lb.Name)
		if !driver.Spec.AcceptDryRunCall {
			return dryRunResponse()
		}
	}
	rsp, err := a.webhookInvoker.CallValidateLoadBalancer(driver, req)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("call webhook error, webhook: validateLoadBalancer, err: %v", err))
	} else if !rsp.Succ {
		return toAdmissionResponse(fmt.Errorf("invalid LoadBalancer: %s", rsp.Msg))
	}
	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateLoadBalancerUpdate implements ValidatingWebHook for LoadBalancer updating
func (a *Admitter) ValidateLoadBalancerUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &lbcfapi.LoadBalancer{}
	oldObj := &lbcfapi.LoadBalancer{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancer failed: %v", err))
	}
	if allowed, msg := LBUpdatedFieldsAllowed(curObj, oldObj); !allowed {
		return toAdmissionResponse(fmt.Errorf(msg))
	}

	errList := ValidateLoadBalancer(curObj)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	driverNamespace := util.NamespaceOfSharedObj(curObj.Spec.LBDriver, curObj.Namespace)
	driver, err := a.driverLister.LoadBalancerDrivers(driverNamespace).Get(curObj.Spec.LBDriver)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("retrieve driver %s/%s failed: %v", driverNamespace, curObj.Spec.LBDriver, err))
	}

	req := &webhooks.ValidateLoadBalancerRequest{
		LBSpec:        curObj.Spec.LBSpec,
		Operation:     webhooks.OperationUpdate,
		Attributes:    curObj.Spec.Attributes,
		OldAttributes: oldObj.Spec.Attributes,
		DryRun:        a.dryRun,
	}
	if a.dryRun {
		klog.Infof("[dry-run] webhook: validateLoadBalancer, LoadBalancer: %s/%s",
			curObj.Namespace, curObj.Name)
		if !driver.Spec.AcceptDryRunCall {
			return dryRunResponse()
		}
	}
	rsp, err := a.webhookInvoker.CallValidateLoadBalancer(driver, req)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("call webhook error, webhook: validateLoadBalancer, err: %v", err))
	} else if !rsp.Succ {
		return toAdmissionResponse(fmt.Errorf("invalid LoadBalancer: %s", rsp.Msg))
	}
	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateLoadBalancerDelete implements ValidatingWebHook for LoadBalancer deleting
func (a *Admitter) ValidateLoadBalancerDelete(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	lb, err := a.lbLister.LoadBalancers(ar.Request.Namespace).Get(ar.Request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return toAdmissionResponse(
				fmt.Errorf("get LoadBalancer %v in %v failed: %v", ar.Request.Name, ar.Request.Namespace, err))
		}
		return toAdmissionResponse(nil)
	}
	if _, ok := lb.Labels[lbcfapi.LabelDoNotDelete]; ok {
		return toAdmissionResponse(
			fmt.Errorf("LoadBalancer with label %s is not allowed to be deleted, delete the label first if you know what you are doing",
				lbcfapi.LabelDoNotDelete))
	}

	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateDriverCreate implements ValidatingWebHook for LoadBalancerDriver creating
func (a *Admitter) ValidateDriverCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	d := &lbcfapi.LoadBalancerDriver{}
	if err := json.Unmarshal(ar.Request.Object.Raw, d); err != nil {
		klog.Errorf(err.Error())
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}

	errList := ValidateLoadBalancerDriver(d)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateDriverUpdate implements ValidatingWebHook for LoadBalancerDriver updating
func (a *Admitter) ValidateDriverUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &lbcfapi.LoadBalancerDriver{}
	oldObj := &lbcfapi.LoadBalancerDriver{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}

	if allowed, msg := DriverUpdatedFieldsAllowed(curObj, oldObj); !allowed {
		return toAdmissionResponse(fmt.Errorf(msg))
	}
	errList := ValidateLoadBalancerDriver(curObj)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateDriverDelete implements ValidatingWebHook for LoadBalancerDriver deleting
func (a *Admitter) ValidateDriverDelete(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	driver, err := a.driverLister.LoadBalancerDrivers(ar.Request.Namespace).Get(ar.Request.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return toAdmissionResponse(nil)
		}
		return toAdmissionResponse(fmt.Errorf("retrieve LoadBalancerDriver %s/%s failed: %v", ar.Request.Namespace, ar.Request.Name, err))
	}
	if !util.IsDriverDraining(driver) {
		return toAdmissionResponse(fmt.Errorf("LoadBalancerDriver must be label with %s:\"true\" before delete", lbcfapi.DriverDrainingLabel))
	}

	lbList, err := a.listLoadBalancerByDriver(ar.Request.Name, ar.Request.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("unable to list LoadBalancers for driver, err: %v", err))
	} else if len(lbList) > 0 {
		return toAdmissionResponse(fmt.Errorf("all LoadBalancers must be deleted, %d remaining", len(lbList)))
	}

	beList, err := a.listBackendByDriver(ar.Request.Name, ar.Request.Namespace)
	if err != nil {
		return toAdmissionResponse(fmt.Errorf("unable to list BackendRecords for driver, err: %v", err))
	} else if len(beList) > 0 {
		return toAdmissionResponse(fmt.Errorf("all BackendRecord must be deregistered, %d remaining", len(beList)))
	}

	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateBackendGroupCreate implements ValidatingWebHook for BackendGroup creating
func (a *Admitter) ValidateBackendGroupCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	bg := &lbcfapi.BackendGroup{}
	if err := json.Unmarshal(ar.Request.Object.Raw, bg); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode BackendGroup failed: %v", err))
	}
	errList := ValidateBackendGroup(bg)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}
	for _, lb := range bg.Spec.GetLoadBalancers() {
		if err := a.validateBackendGroupCreate(bg, lb); err != nil {
			return toAdmissionResponse(err)
		}
	}
	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateBackendGroupUpdate implements ValidatingWebHook for BackendGroup updating
func (a *Admitter) ValidateBackendGroupUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &lbcfapi.BackendGroup{}
	oldObj := &lbcfapi.BackendGroup{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
	}

	if allowed, msg := BackendGroupUpdateFieldsAllowed(curObj, oldObj); !allowed {
		return toAdmissionResponse(fmt.Errorf(msg))
	}

	errList := ValidateBackendGroup(curObj)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}

	for _, lb := range curObj.Spec.GetLoadBalancers() {
		if err := a.validateBackendGroupUpdate(oldObj, curObj, lb); err != nil {
			return toAdmissionResponse(err)
		}
	}
	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateBackendGroupDelete implements ValidatingWebHook for BackendGroup deleting
func (a *Admitter) ValidateBackendGroupDelete(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	bg, err := a.bgLister.BackendGroups(ar.Request.Namespace).Get(ar.Request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return toAdmissionResponse(
				fmt.Errorf("get BackendGroup %v in %v failed: %v", ar.Request.Name, ar.Request.Namespace, err))
		}
		return toAdmissionResponse(nil)
	}
	if _, ok := bg.Labels[lbcfapi.LabelDoNotDelete]; ok {
		return toAdmissionResponse(
			fmt.Errorf("BackendGroup with label %s is not allowed to be deleted, delete the label first if you know what you are doing",
				lbcfapi.LabelDoNotDelete))
	}

	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(nil)
}

// ValidateBindCreate implements ValidatingWebHook for Bind creating
func (a *Admitter) ValidateBindCreate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	bind := &v1.Bind{}
	if err := json.Unmarshal(ar.Request.Object.Raw, bind); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode Bind failed: %v", err))
	}
	errList := ValidateBind(bind)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}
	if a.dryRun {
		return dryRunResponse()
	}
	return toAdmissionResponse(a.validateBindByDriver(nil, bind))
}

// ValidateBindUpdate implements ValidatingWebHook for Bind updating
func (a *Admitter) ValidateBindUpdate(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	curObj := &v1.Bind{}
	oldObj := &v1.Bind{}

	if err := json.Unmarshal(ar.Request.Object.Raw, curObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode Bind failed: %v", err))
	}
	if err := json.Unmarshal(ar.Request.OldObject.Raw, oldObj); err != nil {
		return toAdmissionResponse(fmt.Errorf("decode Bind failed: %v", err))
	}
	if reflect.DeepEqual(oldObj.Spec, curObj.Spec) {
		return toAdmissionResponse(nil)
	}
	errList := ValidateBind(curObj)
	if len(errList) > 0 {
		return toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
	}
	return toAdmissionResponse(a.validateBindByDriver(oldObj, curObj))
}

// ValidateBindDelete implements ValidatingWebHook for Bind deleting
func (a *Admitter) ValidateBindDelete(ar *admission.AdmissionReview) *admission.AdmissionResponse {
	return toAdmissionResponse(nil)
}

func (a *Admitter) validateBindByDriver(oldBind, curbind *v1.Bind) error {
	wg := sync.WaitGroup{}
	resultChan := make(chan error, len(curbind.Spec.LoadBalancers)*2)
	defer close(resultChan)
	for _, lb := range curbind.Spec.LoadBalancers {
		driverNamespace := util.NamespaceOfSharedObj(lb.Driver, curbind.Namespace)
		driver, err := a.driverLister.LoadBalancerDrivers(driverNamespace).Get(lb.Driver)
		if err != nil {
			return fmt.Errorf("retrieve driver %s/%s failed: %v", driverNamespace, lb.Driver, err)
		}
		if util.IsDriverDraining(driver) {
			return fmt.Errorf("driver %q is draining, all LoadBalancer creating operation for that dirver is denied",
				lb.Driver)
		} else if driver.DeletionTimestamp != nil {
			return fmt.Errorf("driver %q is deleting, all LoadBalancer creating operation for that dirver is denied",
				lb.Driver)
		}
		wg.Add(1)
		go func(lb v1.TargetLoadBalancer, driver *lbcfapi.LoadBalancerDriver) {
			defer wg.Done()
			req := &webhooks.ValidateLoadBalancerRequest{
				LBSpec:     lb.Spec,
				Attributes: lb.Attributes,
				DryRun:     a.dryRun,
			}
			if oldBind != nil {
				req.Operation = webhooks.OperationUpdate
				var oa map[string]string
				for _, oldLB := range oldBind.Spec.LoadBalancers {
					if oldLB.Name == lb.Name {
						oa = oldLB.Attributes
						break
					}
				}
				req.OldAttributes = oa
			} else {
				req.Operation = webhooks.OperationCreate
			}
			rsp, err := a.webhookInvoker.CallValidateLoadBalancer(driver, req)
			if err != nil {
				resultChan <- fmt.Errorf("call webhook validateLoadBalancer for LoadBalancer %s failed: %v",
					lb.Name, err)
			} else if !rsp.Succ {
				resultChan <- fmt.Errorf("LoadBalancer %s is invalid: %s", lb.Name, rsp.Msg)
			} else {
				resultChan <- nil
			}
		}(lb, driver)

		wg.Add(1)
		go func(lb v1.TargetLoadBalancer, driver *lbcfapi.LoadBalancerDriver) {
			defer wg.Done()
			req := &webhooks.ValidateBackendRequest{
				BackendType: string(util.TypePod),
				LBInfo:      lb.Spec,
				Parameters:  curbind.Spec.Parameters,
				DryRun:      a.dryRun,
			}
			if oldBind != nil {
				req.Operation = webhooks.OperationUpdate
				req.OldParameters = oldBind.Spec.Parameters
			} else {
				req.Operation = webhooks.OperationCreate
			}
			rsp, err := a.webhookInvoker.CallValidateBackend(driver, req)
			if err != nil {
				resultChan <- fmt.Errorf("call webhook validateBackend for LoadBalancer %s failed: %v",
					lb.Name, err)
			} else if !rsp.Succ {
				resultChan <- fmt.Errorf("backend parameters for LoadBalancer %s is invalid: %s", lb.Name, rsp.Msg)
			} else {
				resultChan <- nil
			}
		}(lb, driver)
	}
	wg.Wait()
	var errs []error
loop:
	for {
		select {
		case err := <-resultChan:
			if err != nil {
				errs = append(errs, err)
			}
		default:
			break loop
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return util.ErrorList(errs)
}

func (a *Admitter) listLoadBalancerByDriver(driverName string, driverNamespace string) ([]*lbcfapi.LoadBalancer, error) {
	lbList, err := a.lbLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.LoadBalancer
	for _, lb := range lbList {
		if driverNamespace != metav1.NamespaceSystem && lb.Namespace != driverNamespace {
			continue
		}
		if lb.Spec.LBDriver == driverName {
			ret = append(ret, lb)
		}
	}
	return ret, nil
}

func (a *Admitter) listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error) {
	recordList, err := a.backendLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.BackendRecord
	for _, r := range recordList {
		if driverNamespace != metav1.NamespaceSystem && r.Namespace != driverNamespace {
			continue
		}
		if r.Spec.LBDriver == driverName {
			ret = append(ret, r)
		}
	}
	return ret, nil
}

func (a *Admitter) validateBackendGroupCreate(bg *lbcfapi.BackendGroup, lbName string) error {
	lb, err := a.getLBForBackendGroup(lbName, bg)
	if err != nil {
		return err
	}
	driverNamespace := util.NamespaceOfSharedObj(lb.Spec.LBDriver, bg.Namespace)
	driver, err := a.driverLister.LoadBalancerDrivers(driverNamespace).Get(lb.Spec.LBDriver)
	if err != nil {
		return fmt.Errorf("retrieve driver %s/%s failed: %v", driverNamespace, lb.Spec.LBDriver, err)
	}
	if util.IsDriverDraining(driver) {
		return fmt.Errorf("driver %q is draining, all BackendGroup creating operation for that dirver is denied", lb.Spec.LBDriver)
	} else if driver.DeletionTimestamp != nil {
		return fmt.Errorf("driver %q is deleting, all BackendGroup creating operation for that dirver is denied", lb.Spec.LBDriver)
	}
	req := &webhooks.ValidateBackendRequest{
		BackendType: string(util.GetBackendType(bg)),
		LBInfo:      lb.Status.LBInfo,
		Operation:   webhooks.OperationCreate,
		Parameters:  bg.Spec.Parameters,
		DryRun:      a.dryRun,
	}
	if a.dryRun {
		klog.Infof("[dry-run] webhook: validateBackend, BackendGroup: %s/%s",
			bg.Namespace, bg.Name)
		if !driver.Spec.AcceptDryRunCall {
			return nil
		}
	}
	rsp, err := a.webhookInvoker.CallValidateBackend(driver, req)
	if err != nil {
		return fmt.Errorf("call webhook error, webhook validateBackend, err: %v", err)
	} else if !rsp.Succ {
		return fmt.Errorf("invalid Backend, msg: %v", rsp.Msg)
	}
	return nil
}

func (a *Admitter) validateBackendGroupUpdate(oldObj, curObj *lbcfapi.BackendGroup, lbName string) error {
	lb, err := a.getLBForBackendGroup(lbName, curObj)
	if err != nil {
		return err
	}
	driverNamespace := util.NamespaceOfSharedObj(lb.Spec.LBDriver, curObj.Namespace)
	driver, err := a.driverLister.LoadBalancerDrivers(driverNamespace).Get(lb.Spec.LBDriver)
	if err != nil {
		return fmt.Errorf("retrieve driver %s/%s failed: %v", driverNamespace, lb.Spec.LBDriver, err)
	}

	req := &webhooks.ValidateBackendRequest{
		BackendType:   string(util.GetBackendType(curObj)),
		LBInfo:        lb.Status.LBInfo,
		Operation:     webhooks.OperationUpdate,
		Parameters:    curObj.Spec.Parameters,
		OldParameters: oldObj.Spec.Parameters,
		DryRun:        a.dryRun,
	}
	if a.dryRun {
		klog.Infof("[dry-run] webhook: validateBackend, BackendGroup: %s/%s",
			curObj.Namespace, curObj.Name)
		if !driver.Spec.AcceptDryRunCall {
			return nil
		}
	}
	rsp, err := a.webhookInvoker.CallValidateBackend(driver, req)
	if err != nil {
		return fmt.Errorf("call webhook error, webhook validateBackend, err: %v", err)
	} else if !rsp.Succ {
		return fmt.Errorf("invalid Backend, msg: %v", rsp.Msg)
	}
	return nil
}

func (a *Admitter) getLBForBackendGroup(lbName string, bg *lbcfapi.BackendGroup) (*lbcfapi.LoadBalancer, error) {
	lbNamespace := util.NamespaceOfSharedObj(lbName, bg.Namespace)
	lb, err := a.lbLister.LoadBalancers(lbNamespace).Get(lbName)
	if err != nil {
		return nil, fmt.Errorf("loadbalancer not found, LoadBalancer must be created before BackendGroup")
	}
	if lb.DeletionTimestamp != nil {
		return nil, fmt.Errorf("operation denied: loadbalancer %q is deleting", lb.Name)
	}
	if !util.IsLoadBalancerAllowedForBackendGroup(lb, bg.Namespace) {
		return nil, fmt.Errorf("LoadBalancer %s/%s is not allowed in namespace %s", lb.Namespace, lb.Name, bg.Namespace)
	}
	return lb, nil
}
