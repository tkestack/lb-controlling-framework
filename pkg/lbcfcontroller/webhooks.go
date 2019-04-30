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
	"path"

	"git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"github.com/parnurzeal/gorequest"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	WebhookPrefix      = "lbcf"
	ValidateLBHook     = "validateLoadBalancer"
	CreateLBHook       = "createLoadBalancer"
	EnsureLBHook       = "ensureLoadBalancer"
	DeleteLBHook       = "deleteLoadBalancer"
	ValidateBEHook     = "validateBackend"
	GenerateBEAddrHook = "generateBackendAddr"
	EnsureBEHook       = "ensureBackend"
	DeregBEHook        = "deregisterBackend"
	UpdateBEHook       = "updateBackend"
)

var knownWebhooks = sets.NewString(
	ValidateLBHook,
	CreateLBHook,
	EnsureLBHook,
	DeleteLBHook,
	ValidateBEHook,
	GenerateBEAddrHook,
	EnsureBEHook,
	DeregBEHook,
	UpdateBEHook,
)

const (
	OperationCreate = "Create"
	OperationUpdate = "Update"
)

type RequestForRetryHooks struct {
	RecordID string `json:"recordID"`
	RetryID  string `json:"retryID"`
}

type ResponseForNoRetryHooks struct {
	Succ bool   `json:"succ"`
	Msg  string `json:"msg"`
}

const (
	StatusSucc    = "Succ"
	StatusFail    = "Fail"
	StatusRunning = "Running"
)

type ResponseForFailRetryHooks struct {
	Status                 string `json:"status"`
	Msg                    string `json:"msg"`
	MinRetryDelayInSeconds int32  `json:"minRetryDelayInSeconds"`
}

type ValidateLoadBalancerRequest struct {
	LBSpec        map[string]string `json:"lbSpec"`
	Operation     string            `json:"operation"`
	Attributes    map[string]string `json:"attributes"`
	OldAttributes map[string]string `json:"oldAttributes,omitempty"`
}

type ValidateLoadBalancerResponse struct {
	ResponseForNoRetryHooks
}

type CreateLoadBalancerRequest struct {
	RequestForRetryHooks
	LBSpec     map[string]string `json:"lbSpec"`
	Attributes map[string]string `json:"attributes"`
}

type CreateLoadBalancerResponse struct {
	ResponseForFailRetryHooks
	LBInfo map[string]string `json:"lbInfo"`
}

type EnsureLoadBalancerRequest struct {
	RequestForRetryHooks
	LBInfo     map[string]string `json:"lbInfo"`
	Attributes map[string]string `json:"attributes"`
}

type EnsureLoadBalancerResponse struct {
	ResponseForFailRetryHooks
}

type DeleteLoadBalancerRequest struct {
	RequestForRetryHooks
	LBInfo     map[string]string `json:"lbInfo"`
	Attributes map[string]string `json:"attributes"`
}

type DeleteLoadBalancerResponse struct {
	ResponseForFailRetryHooks
}

type ValidateBackendRequest struct {
	BackendType   string            `json:"backendType"`
	LBInfo        map[string]string `json:"lbInfo"`
	Operation     string            `json:"operation"`
	Parameters    map[string]string `json:"parameters"`
	OldParameters map[string]string `json:"OldParameters,omitempty"`
}

type ValidateBackendResponse struct {
	ResponseForNoRetryHooks
}

type GenerateBackendAddrRequest struct {
	RequestForRetryHooks
	PodBackend     *PodBackendInGenerateAddrRequest     `json:"podBackend"`
	ServiceBackend *ServiceBackendInGenerateAddrRequest `json:"serviceBackend"`
}

type PodBackendInGenerateAddrRequest struct {
	Pod  v1.Pod           `json:"pod"`
	Port v1beta1.PortSelector `json:"port"`
}

type ServiceBackendInGenerateAddrRequest struct {
	Service v1.Service     `json:"service"`
	Port    v1.ServicePort `json:"port"`
	Node    v1.Node        `json:"node"`
}

type GenerateBackendAddrResponse struct {
	ResponseForFailRetryHooks
	BackendAddr string `json:"backendAddr"`
}

type BackendOperationRequest struct {
	RequestForRetryHooks
	LBInfo       map[string]string `json:"lbInfo"`
	BackendAddr  string            `json:"backendAddr"`
	Parameters   map[string]string `json:"parameters"`
	InjectedInfo map[string]string `json:"injectedInfo"`
}

type BackendOperationResponse struct {
	RecordID string            `json:"recordID"`
	RetryID  string            `json:"retryID"`
	LBInfo   map[string]string `json:"lbInfo"`
	ResponseForFailRetryHooks
	InjectedInfo map[string]string `json:"injectedInfo"`
}

func callWebhook(driver *v1beta1.LoadBalancerDriver, webHookName string, payload interface{}, rsp interface{}) error {
	u := driver.Spec.Url
	u.Path = path.Join(WebhookPrefix, webHookName)
	timeout := DefaultWebhookTimeout
	for _, h := range driver.Spec.Webhooks {
		if h.Name == webHookName {
			if h.Timeout != nil {
				timeout = h.Timeout.Duration
			}
			break
		}
	}
	_, body, errs := gorequest.New().Timeout(timeout).Post(u.String()).Send(payload).EndBytes()
	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	if err := json.Unmarshal(body, rsp); err != nil {
		return err
	}
	return nil
}

