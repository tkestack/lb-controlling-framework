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
	"net/http"
	"net/url"
	"path"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	"github.com/parnurzeal/gorequest"
	"k8s.io/klog"
)

const (
	// DefaultWebhookTimeout is the default timeout of calling webhooks
	DefaultWebhookTimeout = 10 * time.Second
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
	rsp := &webhooks.ValidateLoadBalancerResponse{}
	if err := callWebhook(driver, webhooks.ValidateLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallCreateLoadBalancer calls webhook createLoadBalancer on driver
func (w *WebhookInvokerImpl) CallCreateLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	rsp := &webhooks.CreateLoadBalancerResponse{}
	if err := callWebhook(driver, webhooks.CreateLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallEnsureLoadBalancer calls webhook ensureLoadBalancer on driver
func (w *WebhookInvokerImpl) CallEnsureLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	rsp := &webhooks.EnsureLoadBalancerResponse{}
	if err := callWebhook(driver, webhooks.EnsureLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallDeleteLoadBalancer calls webhook deleteLoadBalancer on driver
func (w *WebhookInvokerImpl) CallDeleteLoadBalancer(driver *lbcfapi.LoadBalancerDriver, req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	rsp := &webhooks.DeleteLoadBalancerResponse{}
	if err := callWebhook(driver, webhooks.DeleteLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallValidateBackend calls webhook validateBackend on driver
func (w *WebhookInvokerImpl) CallValidateBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	rsp := &webhooks.ValidateBackendResponse{}
	if err := callWebhook(driver, webhooks.ValidateBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallGenerateBackendAddr calls webhook generateBackendAddr on driver
func (w *WebhookInvokerImpl) CallGenerateBackendAddr(driver *lbcfapi.LoadBalancerDriver, req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	rsp := &webhooks.GenerateBackendAddrResponse{}
	if err := callWebhook(driver, webhooks.GenerateBackendAddr, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallEnsureBackend calls webhook ensureBackend on driver
func (w *WebhookInvokerImpl) CallEnsureBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	rsp := &webhooks.BackendOperationResponse{}
	if err := callWebhook(driver, webhooks.EnsureBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

// CallDeregisterBackend calls webhook deregisterBackend on driver
func (w *WebhookInvokerImpl) CallDeregisterBackend(driver *lbcfapi.LoadBalancerDriver, req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	rsp := &webhooks.BackendOperationResponse{}
	if err := callWebhook(driver, webhooks.DeregBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func callWebhook(driver *lbcfapi.LoadBalancerDriver, webHookName string, payload interface{}, rsp interface{}) error {
	u, err := url.Parse(driver.Spec.Url)
	if err != nil {
		e := fmt.Errorf("invalid url: %v", err)
		klog.Errorf("callwebhook failed: %v. driver: %s, webhookName: %s", e, driver.Name, webHookName)
		return e
	}
	u.Path = path.Join(webHookName)
	timeout := DefaultWebhookTimeout
	for _, h := range driver.Spec.Webhooks {
		if h.Name == webHookName {
			if h.Timeout != nil {
				timeout = h.Timeout.Duration
			}
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
