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
	"fmt"
	"net/http"

	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"

	"github.com/emicklei/go-restful"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// NewWebhookServer creates a new Server
func NewWebhookServer(context *context.Context, crtFile string, keyFile string) *Server {
	s := &Server{
		context:      context,
		admitWebhook: NewAdmitter(context.LBInformer.Lister(), context.LBDriverInformer.Lister(), context.BRInformer.Lister(), util.NewWebhookInvoker()),
		crtFile:      crtFile,
		keyFile:      keyFile,
	}
	return s
}

// Server is the server that provides Kubernetes Admission Webhooks
type Server struct {
	context      *context.Context
	admitWebhook Webhook
	crtFile      string
	keyFile      string
}

// Start starts the server in a new goroutine
func (s *Server) Start() {
	ws := new(restful.WebService)
	ws.Path("/")

	ws.Route(ws.POST("mutate-load-balancer").To(s.MutateAdmitLoadBalancer).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("mutate-load-balancer-driver").To(s.MutateAdmitLoadBalancerDriver).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("mutate-backend-broup").To(s.MutateAdmitBackendGroup).
		Consumes(restful.MIME_JSON))

	ws.Route(ws.POST("validate-load-balancer").To(s.ValidateAdmitLoadBalancer).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("validate-load-balancer-driver").To(s.ValidateAdmitLoadBalancerDriver).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("validate-backend-group").To(s.ValidateAdmitBackendGroup).
		Consumes(restful.MIME_JSON))

	restful.Add(ws)

	go func() {
		s.context.WaitForCacheSync()
		klog.Fatal(http.ListenAndServeTLS(":443", s.crtFile, s.keyFile, nil))
	}()
}

// ValidateAdmitLoadBalancer implements ValidatingWebHook for LoadBalancer
func (s *Server) ValidateAdmitLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveValidate(req, rsp, s.admitWebhook.ValidateLoadBalancerCreate, s.admitWebhook.ValidateLoadBalancerUpdate, s.admitWebhook.ValidateLoadBalancerDelete)
}

// ValidateAdmitLoadBalancerDriver implements ValidatingWebHook for LoadBalancerDriver
func (s *Server) ValidateAdmitLoadBalancerDriver(req *restful.Request, rsp *restful.Response) {
	serveValidate(req, rsp, s.admitWebhook.ValidateDriverCreate, s.admitWebhook.ValidateDriverUpdate, s.admitWebhook.ValidateDriverDelete)
}

// ValidateAdmitBackendGroup implements ValidatingWebHook for BackendGroup
func (s *Server) ValidateAdmitBackendGroup(req *restful.Request, rsp *restful.Response) {
	serveValidate(req, rsp, s.admitWebhook.ValidateBackendGroupCreate, s.admitWebhook.ValidateBackendGroupUpdate, s.admitWebhook.ValidateBackendGroupDelete)
}

// MutateAdmitLoadBalancer implements MutatingWebHook for LoadBalancer
func (s *Server) MutateAdmitLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveMutate(req, rsp, s.admitWebhook.MutateLB)
}

// MutateAdmitLoadBalancerDriver implements MutatingWebHook for LoadBalancerDriver
func (s *Server) MutateAdmitLoadBalancerDriver(req *restful.Request, rsp *restful.Response) {
	serveMutate(req, rsp, s.admitWebhook.MutateDriver)
}

// MutateAdmitBackendGroup implements MutatingWebHook for BackendGroup
func (s *Server) MutateAdmitBackendGroup(req *restful.Request, rsp *restful.Response) {
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
