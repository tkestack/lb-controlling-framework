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

package util

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"
	"tkestack.io/lb-controlling-framework/pkg/metrics"

	"github.com/parnurzeal/gorequest"
	"k8s.io/klog"
)

// WebhookInvoker is an abstract interface for testability
type WebhookInvoker interface {
	CallValidateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error)

	CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error)

	CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error)

	CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error)

	CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error)

	CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error)

	CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error)

	CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error)
}

// NewWebhookInvoker creates a new instance of WebhookInvoker
func NewWebhookInvoker() WebhookInvoker {
	return &WebhookInvokerImpl{}
}

// WebhookInvokerImpl is an implementation of WebhookInvoker
type WebhookInvokerImpl struct{}

// CallValidateLoadBalancer calls webhook validateLoadBalancer on driver
func (w *WebhookInvokerImpl) CallValidateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateLoadBalancer")
	rsp := &webhooks.ValidateLoadBalancerResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.ValidateLoadBalancer, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateLoadBalancer")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateLoadBalancer", time.Since(start))
	if !rsp.Succ {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateLoadBalancer")
	}
	return rsp, nil
}

// CallCreateLoadBalancer calls webhook createLoadBalancer on driver
func (w *WebhookInvokerImpl) CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "createLoadBalancer")
	rsp := &webhooks.CreateLoadBalancerResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.CreateLoadBalancer, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "createLoadBalancer")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "createLoadBalancer", time.Since(start))
	if rsp.Status == webhooks.StatusFail {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "createLoadBalancer")
	}
	return rsp, nil
}

// CallEnsureLoadBalancer calls webhook ensureLoadBalancer on driver
func (w *WebhookInvokerImpl) CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureLoadBalancer")
	rsp := &webhooks.EnsureLoadBalancerResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.EnsureLoadBalancer, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureLoadBalancer")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureLoadBalancer", time.Since(start))
	if rsp.Status == webhooks.StatusFail {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureLoadBalancer")
	}
	return rsp, nil
}

// CallDeleteLoadBalancer calls webhook deleteLoadBalancer on driver
func (w *WebhookInvokerImpl) CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deleteLoadBalancer")
	rsp := &webhooks.DeleteLoadBalancerResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.DeleteLoadBalancer, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deleteLoadBalancer")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deleteLoadBalancer", time.Since(start))
	if rsp.Status == webhooks.StatusFail {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deleteLoadBalancer")
	}
	return rsp, nil
}

// CallValidateBackend calls webhook validateBackend on driver
func (w *WebhookInvokerImpl) CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateBackend")
	rsp := &webhooks.ValidateBackendResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.ValidateBackend, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateBackend")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateBackend", time.Since(start))
	if !rsp.Succ {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "validateBackend")
	}
	return rsp, nil
}

// CallGenerateBackendAddr calls webhook generateBackendAddr on driver
func (w *WebhookInvokerImpl) CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "generateBackendAddr")
	rsp := &webhooks.GenerateBackendAddrResponse{}
	// In lbcf v1.1.x and before, we use portNumber instead of port in the CRD and request.
	// The portNumber in CRD is deprecated, but the portNumber in the request is kept so that old drivers can still read it.
	if req.PodBackend != nil {
		req.PodBackend.Port.PortNumber = &req.PodBackend.Port.Port
	} else if req.ServiceBackend != nil {
		req.ServiceBackend.Port.PortNumber = &req.ServiceBackend.Port.Port
	}
	start := time.Now()
	if err := callWebhook(driver, webhooks.GenerateBackendAddr, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "generateBackendAddr")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "generateBackendAddr", time.Since(start))
	if rsp.Status == webhooks.StatusFail {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "generateBackendAddr")
	}
	return rsp, nil
}

// CallEnsureBackend calls webhook ensureBackend on driver
func (w *WebhookInvokerImpl) CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureBackend")
	rsp := &webhooks.BackendOperationResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.EnsureBackend, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureBackend")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureBackend", time.Since(start))
	if rsp.Status == webhooks.StatusFail {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "ensureBackend")
	}
	return rsp, nil
}

// CallDeregisterBackend calls webhook deregisterBackend on driver
func (w *WebhookInvokerImpl) CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	metrics.WebhookCallsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deregisterBackend")
	rsp := &webhooks.BackendOperationResponse{}
	start := time.Now()
	if err := callWebhook(driver, webhooks.DeregBackend, req, rsp); err != nil {
		metrics.WebhookErrorsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deregisterBackend")
		return nil, err
	}
	metrics.WebhookLatencyObserve(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deregisterBackend", time.Since(start))
	if rsp.Status == webhooks.StatusFail {
		metrics.WebhookFailsInc(NamespacedNameKeyFunc(driver.Namespace, driver.Name), "deregisterBackend")
	}
	return rsp, nil
}

func callWebhook(driver *lbcfapi.LoadBalancerDriver, webHookName string, payload interface{}, rsp interface{}) error {
	u, err := url.Parse(driver.Spec.URL)
	if err != nil {
		e := fmt.Errorf("invalid url: %v", err)
		klog.Errorf("callwebhook failed: %v. driver: %s, webhookName: %s", e, driver.Name, webHookName)
		return e
	}
	u.Path = path.Join(webHookName)
	timeout := 10 * time.Second
	for _, h := range driver.Spec.Webhooks {
		if h.Name == webHookName {
			timeout = h.Timeout.Duration
			break
		}
	}
	request := gorequest.New().Timeout(timeout).Post(u.String()).Send(payload)
	debugInfo, _ := request.AsCurlCommand()
	klog.V(3).Infof("callwebhook, %s", debugInfo)

	response, body, errs := request.EndBytes()
	if len(errs) > 0 {
		e := fmt.Errorf("webhook err: %v", errs)
		klog.Errorf("callwebhook failed: %v. url: %s", e, u.String())
		return e
	}
	if response.StatusCode != http.StatusOK {
		e := fmt.Errorf("http status code: %d, body: %s", response.StatusCode, body)
		klog.Errorf("callwebhook failed: %v. url: %s", e, u.String())
		return e
	}
	if err := json.Unmarshal(body, rsp); err != nil {
		e := fmt.Errorf("decode webhook response err: %v, raw: %s", err, body)
		klog.Errorf("callwebhook failed: %v. url: %s", e, u.String())
		return e
	}
	return nil
}
