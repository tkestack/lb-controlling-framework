package lbcfcontroller

import (
	"encoding/json"
	"fmt"

	v1beta12 "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"github.com/emicklei/go-restful"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func (c *Controller) ValidateAdmitLoadBalancer(req *restful.Request, rsp *restful.Response) {
	ar := &v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{},
	}
	if err := req.ReadEntity(ar); err != nil {
		ar.Response = toAdmissionResponse(fmt.Errorf("decode AdmissionReview failed: %v", err))
		if err := rsp.WriteAsJson(ar); err != nil {
			klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
		}
		return
	}

	lb := v1beta12.LoadBalancer{}
	if !decodeObjectSucc(ar.Request.Object.Raw, lb, ar, rsp){
		return
	}

	if _, ok := c.driverManager.getDriver(lb.Spec.LBType, string(LBDriverWebhook)); !ok{
		ar.Response = toAdmissionResponse(fmt.Errorf("LoadBalancer %s not found", lb.Spec.LBType))
		if err := rsp.WriteAsJson(ar); err != nil {
			klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
		}
		return
	}

	// TODO: call driver.valiateLoadBalancer here

}

func (c *Controller) ValidateAdmitLoadBalancerDriver(req *restful.Request, rsp *restful.Response) {
	ar := &v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{},
	}
	if err := req.ReadEntity(ar); err != nil {
		ar.Response = toAdmissionResponse(fmt.Errorf("decode AdmissionReview failed: %v", err))
		if err := rsp.WriteAsJson(ar); err != nil {
			klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
		}
		return
	}
	driver := &v1beta12.LoadBalancerDriver{}
	if !decodeObjectSucc(ar.Request.Object.Raw, driver, ar, rsp){
		return
	}
	errList := ValidateLoadBalancerDriver(driver)
	if len(errList) > 0 {
		ar.Response = toAdmissionResponse(fmt.Errorf("%s", errList.ToAggregate().Error()))
		if err := rsp.WriteAsJson(ar); err != nil {
			klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
		}
		return
	}

	ar.Response.Allowed = true
	if err := rsp.WriteAsJson(ar); err != nil {
		klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
	}
}

func (c *Controller) ValidateAdmitBackendGroup(req *restful.Request, rsp *restful.Response) {

}

func (c *Controller) ValidateAdmitBackendRecord(req *restful.Request, rsp *restful.Response) {

}

func (c *Controller) MutateAdmitLoadBalancer(req *restful.Request, rsp *restful.Response) {

}

func (c *Controller) MutateAdmitLoadBalancerDriver(req *restful.Request, rsp *restful.Response) {
	ar := &v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{},
	}
	if err := req.ReadEntity(ar); err != nil {
		ar.Response = toAdmissionResponse(fmt.Errorf("decode AdmissionReview failed: %v", err))
		if err := rsp.WriteAsJson(ar); err != nil {
			klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
		}
		return
	}
	ar.Response.Allowed = true
	if err := rsp.WriteAsJson(ar); err != nil {
		klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
	}
}

func (c *Controller) MutateAdmitBackendGroup(req *restful.Request, rsp *restful.Response) {
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{Result: &v1.Status{Message: err.Error()}}
}

func decodeObjectSucc(data []byte, v interface{}, ar *v1beta1.AdmissionReview, rsp *restful.Response) bool{
	if err := json.Unmarshal(data, v); err != nil {
		ar.Response = toAdmissionResponse(fmt.Errorf("decode LoadBalancerDriver failed: %v", err))
		if err := rsp.WriteAsJson(ar); err != nil {
			klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
		}
		return false
	}
	return true
}