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
	"fmt"
	"net/http"

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"

	"github.com/emicklei/go-restful"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func NewAdmitServer(context *context.Context, crtFile string, keyFile string) *AdmitServer {
	s := &AdmitServer{
		admitWebhook: NewAdmitter(context.LBInformer.Lister(), context.LBDriverInformer.Lister(), context.BRInformer.Lister(), util.NewWebhookInvoker()),
		crtFile:      crtFile,
		keyFile:      keyFile,
	}
	return s
}

type AdmitServer struct {
	admitWebhook AdmitWebhook
	crtFile      string
	keyFile      string
}

func (s *AdmitServer) Start() {
	ws := new(restful.WebService)
	ws.Path("/")

	ws.Route(ws.POST("mutateLoadBalancer").To(s.MutateAdmitLoadBalancer).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("mutateLoadBalancerDriver").To(s.MutateAdmitLoadBalancerDriver).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("mutateBackendGroup").To(s.MutateAdmitBackendGroup).
		Consumes(restful.MIME_JSON))

	ws.Route(ws.POST("validateLoadBalancer").To(s.ValidateAdmitLoadBalancer).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("validateLoadBalancerDriver").To(s.ValidateAdmitLoadBalancerDriver).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("validateBackendGroup").To(s.ValidateAdmitBackendGroup).
		Consumes(restful.MIME_JSON))

	restful.Add(ws)

	go func() {
		klog.Fatal(http.ListenAndServeTLS(":443", s.crtFile, s.keyFile, nil))
	}()
}

func (s *AdmitServer) ValidateAdmitLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveValidate(req, rsp, s.admitWebhook.ValidateLoadBalancerCreate, s.admitWebhook.ValidateLoadBalancerUpdate, s.admitWebhook.ValidateLoadBalancerDelete)
}

func (s *AdmitServer) ValidateAdmitLoadBalancerDriver(req *restful.Request, rsp *restful.Response) {
	serveValidate(req, rsp, s.admitWebhook.ValidateDriverCreate, s.admitWebhook.ValidateDriverUpdate, s.admitWebhook.ValidateDriverDelete)
}

func (s *AdmitServer) ValidateAdmitBackendGroup(req *restful.Request, rsp *restful.Response) {
	serveValidate(req, rsp, s.admitWebhook.ValidateBackendGroupCreate, s.admitWebhook.ValidateBackendGroupUpdate, s.admitWebhook.ValidateBackendGroupDelete)
}

func (s *AdmitServer) MutateAdmitLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveMutate(req, rsp, s.admitWebhook.MutateLB)
}

func (s *AdmitServer) MutateAdmitLoadBalancerDriver(req *restful.Request, rsp *restful.Response) {
	serveMutate(req, rsp, s.admitWebhook.MutateDriver)
}

func (s *AdmitServer) MutateAdmitBackendGroup(req *restful.Request, rsp *restful.Response) {
	serveMutate(req, rsp, s.admitWebhook.MutateBackendGroup)
}

func parseAdmissionReview(req *restful.Request, rsp *restful.Response) *v1beta1.AdmissionReview {
	ar := &v1beta1.AdmissionReview{}
	if err := req.ReadEntity(ar); err != nil {
		ar.Response = toAdmissionResponse(fmt.Errorf("decode AdmissionReview failed: %v", err))
		responseAndLog(ar, rsp)
		return nil
	}
	return ar
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	if err == nil {
		return &v1beta1.AdmissionResponse{Allowed: true}
	}
	return &v1beta1.AdmissionResponse{Result: &v1.Status{Message: err.Error()}}
}

func responseAndLog(ar *v1beta1.AdmissionReview, rsp *restful.Response) {
	if err := rsp.WriteAsJson(ar); err != nil {
		klog.Errorf("send admissionWebhook response failed: %v, ar: %+v", err, *ar)
	}
}

func serveValidate(req *restful.Request, rsp *restful.Response, createFunc admitFunc, updateFunc admitFunc, deleteFunc admitFunc) {
	ar := parseAdmissionReview(req, rsp)
	if ar == nil {
		return
	}
	responseAndLog(validate(ar, createFunc, updateFunc, deleteFunc), rsp)
}

func serveMutate(req *restful.Request, rsp *restful.Response, mutateFunc admitFunc) {
	ar := parseAdmissionReview(req, rsp)
	if ar == nil {
		return
	}
	responseAndLog(mutate(ar, mutateFunc), rsp)
}

type admitFunc func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

func validate(requestAdmissionReview *v1beta1.AdmissionReview, createFunc admitFunc, updateFunc admitFunc, deleteFunc admitFunc) *v1beta1.AdmissionReview {
	responseAdmissionReview := &v1beta1.AdmissionReview{}
	switch requestAdmissionReview.Request.Operation {
	case v1beta1.Create:
		responseAdmissionReview.Response = createFunc(requestAdmissionReview)
	case v1beta1.Update:
		responseAdmissionReview.Response = updateFunc(requestAdmissionReview)
	case v1beta1.Delete:
		responseAdmissionReview.Response = deleteFunc(requestAdmissionReview)
	default:
		responseAdmissionReview.Response = toAdmissionResponse(nil)
	}
	responseAdmissionReview.Response.UID = requestAdmissionReview.Request.UID
	return responseAdmissionReview
}

func mutate(requestAdmissionReview *v1beta1.AdmissionReview, mutateFunc admitFunc) *v1beta1.AdmissionReview {
	responseAdmissionReview := &v1beta1.AdmissionReview{}
	responseAdmissionReview.Response = mutateFunc(requestAdmissionReview)
	responseAdmissionReview.Response.UID = requestAdmissionReview.Request.UID
	return responseAdmissionReview
}
