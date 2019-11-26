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

package util

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhooksSucc(t *testing.T) {
	invoker := NewWebhookInvoker()

	succServer := newMockServer(succValidate, succRun)
	u, err := succServer.start()
	if err != nil {
		t.Fatalf(err.Error())
	}
	rsp, err := invoker.CallValidateLoadBalancer(fakeMockDriver(u, 10*time.Second), &webhooks.ValidateLoadBalancerRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !rsp.Succ {
		t.Errorf("CallValidateLoadBalancer fail")
	} else if rsp.Msg != "fake-validate-succ" {
		t.Errorf("CallValidateLoadBalancer get Msg %s", rsp.Msg)
	}

	createLBRsp, err := invoker.CallCreateLoadBalancer(fakeMockDriver(u, 10*time.Second), &webhooks.CreateLoadBalancerRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if createLBRsp.Status != webhooks.StatusSucc {
		t.Errorf("CallCreateLoadBalancer fail")
	} else if createLBRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallCreateLoadBalancer get empty Msg")
	}

	ensureLBRsp, err := invoker.CallEnsureLoadBalancer(fakeMockDriver(u, 10*time.Second), &webhooks.EnsureLoadBalancerRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if ensureLBRsp.Status != webhooks.StatusSucc {
		t.Errorf("CallEnsureLoadBalancer fail")
	} else if ensureLBRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallEnsureLoadBalancer get empty Msg")
	}

	deleteLBRsp, err := invoker.CallDeleteLoadBalancer(fakeMockDriver(u, 10*time.Second), &webhooks.DeleteLoadBalancerRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if deleteLBRsp.Status != webhooks.StatusSucc {
		t.Errorf("CallEnsureLoadBalancer fail")
	} else if deleteLBRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallEnsureLoadBalancer get empty Msg")
	}

	validateBeRsp, err := invoker.CallValidateBackend(fakeMockDriver(u, 10*time.Second), &webhooks.ValidateBackendRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !validateBeRsp.Succ {
		t.Errorf("CallValidateBackend fail")
	} else if validateBeRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallValidateBackend get empty Msg")
	}

	generateLBRsp, err := invoker.CallGenerateBackendAddr(fakeMockDriver(u, 10*time.Second), &webhooks.GenerateBackendAddrRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if generateLBRsp.Status != webhooks.StatusSucc {
		t.Errorf("CallGenerateBackendAddr fail")
	} else if generateLBRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallGenerateBackendAddr get empty Msg")
	}

	ensureBeRsp, err := invoker.CallEnsureBackend(fakeMockDriver(u, 10*time.Second), &webhooks.BackendOperationRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if ensureBeRsp.Status != webhooks.StatusSucc {
		t.Errorf("CallEnsureBackend fail")
	} else if ensureBeRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallEnsureBackend get empty Msg")
	}

	deregBeRsp, err := invoker.CallDeregisterBackend(fakeMockDriver(u, 10*time.Second), &webhooks.BackendOperationRequest{})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if deregBeRsp.Status != webhooks.StatusSucc {
		t.Errorf("CallDeregisterBackend fail")
	} else if deregBeRsp.Msg != "fake-validate-succ" {
		t.Errorf("CallDeregisterBackend get empty Msg")
	}
}

func TestWebhookTimeout(t *testing.T) {
	invoker := NewWebhookInvoker()

	failServer := newMockServer(timeoutlValidate, timeoutRun)
	u, err := failServer.start()
	if err != nil {
		t.Fatalf(err.Error())
	}
	_, err = invoker.CallValidateLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.ValidateLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallCreateLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.CreateLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallEnsureLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.EnsureLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallDeleteLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.DeleteLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallValidateBackend(fakeMockDriver(u, 500*time.Millisecond), &webhooks.ValidateBackendRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallGenerateBackendAddr(fakeMockDriver(u, 500*time.Millisecond), &webhooks.GenerateBackendAddrRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallEnsureBackend(fakeMockDriver(u, 500*time.Millisecond), &webhooks.BackendOperationRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallDeregisterBackend(fakeMockDriver(u, 500*time.Millisecond), &webhooks.BackendOperationRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expect timeout, get %v", err)
	}
}

func TestWebhookHttpErr(t *testing.T) {
	invoker := NewWebhookInvoker()

	failServer := newMockServer(httpErrValidate, httpErrRun)
	u, err := failServer.start()
	if err != nil {
		t.Fatalf(err.Error())
	}
	_, err = invoker.CallValidateLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.ValidateLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallCreateLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.CreateLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallEnsureLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.EnsureLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallDeleteLoadBalancer(fakeMockDriver(u, 500*time.Millisecond), &webhooks.DeleteLoadBalancerRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallValidateBackend(fakeMockDriver(u, 500*time.Millisecond), &webhooks.ValidateBackendRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallGenerateBackendAddr(fakeMockDriver(u, 500*time.Millisecond), &webhooks.GenerateBackendAddrRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallEnsureBackend(fakeMockDriver(u, 500*time.Millisecond), &webhooks.BackendOperationRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}

	_, err = invoker.CallDeregisterBackend(fakeMockDriver(u, 500*time.Millisecond), &webhooks.BackendOperationRequest{})
	if err == nil {
		t.Errorf("expect timeout")
	} else if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expect timeout, get %v", err)
	}
}

func succValidate(rsp http.ResponseWriter, req *http.Request) {
	body := &webhooks.ResponseForNoRetryHooks{
		Succ: true,
		Msg:  "fake-validate-succ",
	}
	payLoad, _ := json.Marshal(body)
	rsp.Write(payLoad)
}

func failValidate(rsp http.ResponseWriter, req *http.Request) {
	body := &webhooks.ResponseForNoRetryHooks{
		Succ: false,
		Msg:  "fake-validate-fail",
	}
	payLoad, _ := json.Marshal(body)
	rsp.Write(payLoad)
}

func timeoutlValidate(rsp http.ResponseWriter, req *http.Request) {
	time.Sleep(2 * time.Second)
	succValidate(rsp, req)
}

func httpErrValidate(rsp http.ResponseWriter, req *http.Request) {
	rsp.WriteHeader(http.StatusNotFound)
}

func succRun(rsp http.ResponseWriter, req *http.Request) {
	body := &webhooks.ResponseForFailRetryHooks{
		Status: webhooks.StatusSucc,
		Msg:    "fake-validate-succ",
	}
	payLoad, _ := json.Marshal(body)
	rsp.Write(payLoad)
}

func failRun(rsp http.ResponseWriter, req *http.Request) {
	body := &webhooks.ResponseForFailRetryHooks{
		Status:                 webhooks.StatusFail,
		Msg:                    "fake-validate-succ",
		MinRetryDelayInSeconds: 30,
	}
	payLoad, _ := json.Marshal(body)
	rsp.Write(payLoad)
}

func timeoutRun(rsp http.ResponseWriter, req *http.Request) {
	time.Sleep(2 * time.Second)
	succRun(rsp, req)
}

func httpErrRun(rsp http.ResponseWriter, req *http.Request) {
	rsp.WriteHeader(http.StatusNotFound)
}

func newMockServer(validateFunc func(http.ResponseWriter, *http.Request), runFunc func(http.ResponseWriter, *http.Request)) *mockServer {
	mux := http.NewServeMux()

	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.ValidateLoadBalancer), validateFunc)
	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.CreateLoadBalancer), runFunc)
	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.EnsureLoadBalancer), runFunc)
	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.DeleteLoadBalancer), runFunc)

	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.ValidateBackend), validateFunc)
	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.GenerateBackendAddr), runFunc)
	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.EnsureBackend), runFunc)
	mux.HandleFunc(fmt.Sprintf("/%s", webhooks.DeregBackend), runFunc)

	return &mockServer{
		mux: mux,
	}
}

type mockServer struct {
	mux      *http.ServeMux
	listener net.Listener
}

func (s *mockServer) start() (*url.URL, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.listener = listener
	started := make(chan struct{})
	go func() {
		close(started)
		http.Serve(s.listener, s.mux)
	}()
	<-started
	addr := s.listener.Addr().String()
	return url.Parse(fmt.Sprintf("http://%s", addr))
}

func fakeMockDriver(u *url.URL, timeout time.Duration) *lbcfapi.LoadBalancerDriver {
	return &lbcfapi.LoadBalancerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-driver",
		},
		Spec: lbcfapi.LoadBalancerDriverSpec{
			DriverType: string(lbcfapi.WebhookDriver),
			Url:        u.String(),
			Webhooks: []lbcfapi.WebhookConfig{
				{
					Name: webhooks.ValidateLoadBalancer,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.CreateLoadBalancer,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.EnsureLoadBalancer,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.DeleteLoadBalancer,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.ValidateBackend,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.GenerateBackendAddr,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.EnsureBackend,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
				{
					Name: webhooks.DeregBackend,
					Timeout: lbcfapi.Duration{
						Duration: timeout,
					},
				},
			},
		},
	}
}
