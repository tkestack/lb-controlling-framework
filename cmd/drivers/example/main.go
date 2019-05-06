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

package main

import (
	"flag"
	"fmt"
	"net/http"

	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	"github.com/emicklei/go-restful"
	"k8s.io/klog"
)

func main() {
	klog.InitFlags(flag.NewFlagSet("example-driver", flag.ContinueOnError))
	flag.Set("logtostderr", "true")
	defer klog.Flush()

	ws := new(restful.WebService)
	ws.Path("/lbcf")

	ws.Route(ws.POST(webhooks.ValidateLoadBalancer).To(ValidateLoadBalancer))
	ws.Route(ws.POST(webhooks.CreateLoadBalancer).To(CreateLoadBalancer))
	ws.Route(ws.POST(webhooks.EnsureLoadBalancer).To(EnsureLoadBalancer))
	ws.Route(ws.POST(webhooks.DeleteLoadBalancer).To(DeleteLoadBalancer))

	ws.Route(ws.POST(webhooks.ValidateBackend).To(ValidateBackend))
	ws.Route(ws.POST(webhooks.GenerateBackendAddr).To(GenerateBackendAddr))
	ws.Route(ws.POST(webhooks.EnsureBackend).To(EnsureBackend))
	ws.Route(ws.POST(webhooks.DeregBackend).To(DeregBackend))

	restful.Add(ws)
	http.ListenAndServe(":11029", nil)
}

func ValidateLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveNoRetry(req, rsp, &webhooks.ValidateLoadBalancerRequest{})
}

func CreateLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveRetry(req, rsp, &webhooks.CreateLoadBalancerRequest{})
}

func EnsureLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveRetry(req, rsp, &webhooks.EnsureLoadBalancerRequest{})
}

func DeleteLoadBalancer(req *restful.Request, rsp *restful.Response) {
	serveRetry(req, rsp, &webhooks.DeleteLoadBalancerRequest{})
}

func ValidateBackend(req *restful.Request, rsp *restful.Response) {
	serveNoRetry(req, rsp, &webhooks.ValidateBackendRequest{})
}

func GenerateBackendAddr(req *restful.Request, rsp *restful.Response) {
	obj := &webhooks.GenerateBackendAddrRequest{}
	rspPayload := &webhooks.GenerateBackendAddrResponse{}
	if err := req.ReadEntity(obj); err != nil {
		klog.Infof("err: %v", err)
		rspPayload.Msg = err.Error()
		rsp.WriteAsJson(rspPayload)
		return
	}
	klog.Infof("receive request url: %s, %+v", req.Request.URL.String(), obj)
	rspPayload.Status = webhooks.StatusSucc
	rspPayload.BackendAddr = fmt.Sprintf("%s:%d", obj.PodBackend.Pod.Name, obj.PodBackend.Port.PortNumber)
	rsp.WriteAsJson(rspPayload)
	klog.Infof("finished")
}

func EnsureBackend(req *restful.Request, rsp *restful.Response) {
	serveRetry(req, rsp, &webhooks.BackendOperationRequest{})
}

func DeregBackend(req *restful.Request, rsp *restful.Response) {
	serveRetry(req, rsp, &webhooks.BackendOperationRequest{})
}

func serveNoRetry(req *restful.Request, rsp *restful.Response, obj interface{}) {
	rspPayload := &webhooks.ResponseForNoRetryHooks{}
	if err := req.ReadEntity(obj); err != nil {
		klog.Infof("err: %v", err)
		rspPayload.Msg = err.Error()
		rsp.WriteAsJson(rspPayload)
		return
	}
	klog.Infof("receive request url: %s, %+v", req.Request.URL.String(), obj)
	rspPayload.Succ = true
	rsp.WriteAsJson(rspPayload)
	klog.Infof("finished")
}

func serveRetry(req *restful.Request, rsp *restful.Response, obj interface{}) {
	rspPayload := &webhooks.ResponseForFailRetryHooks{}
	if err := req.ReadEntity(obj); err != nil {
		klog.Infof("err: %v", err)
		rspPayload.Msg = err.Error()
		rsp.WriteAsJson(rspPayload)
		return
	}
	klog.Infof("receive request url: %s, %+v", req.Request.URL.String(), obj)
	rspPayload.Status = webhooks.StatusSucc
	rsp.WriteAsJson(rspPayload)
	klog.Infof("finished")
}
