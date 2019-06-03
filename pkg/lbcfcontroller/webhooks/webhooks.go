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

package webhooks

import (
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// ValidateLoadBalancer is the name and URL path of webhook validateLoadBalancer
	ValidateLoadBalancer = "validateLoadBalancer"

	// CreateLoadBalancer is the name and URL path of webhook createLoadBalancer
	CreateLoadBalancer = "createLoadBalancer"

	// EnsureLoadBalancer is the name and URL path of webhook ensureLoadBalancer
	EnsureLoadBalancer = "ensureLoadBalancer"

	// DeleteLoadBalancer is the name and URL path of webhook deleteLoadBalancer
	DeleteLoadBalancer = "deleteLoadBalancer"

	// ValidateBackend is the name and URL path of webhook validateBackend
	ValidateBackend = "validateBackend"

	// GenerateBackendAddr is the name and URL path of webhook generateBackendAddr
	GenerateBackendAddr = "generateBackendAddr"

	// EnsureBackend is the name and URL path of webhook ensureBackend
	EnsureBackend = "ensureBackend"

	// DeregBackend is the name and URL path of webhook deregisterBackend
	DeregBackend = "deregisterBackend"

	// UpdateBackend is the name and URL path of webhook updateBackend
	UpdateBackend = "updateBackend"
)

// KnownWebhooks is a set contains all supported webhooks
var KnownWebhooks = sets.NewString(
	ValidateLoadBalancer,
	CreateLoadBalancer,
	EnsureLoadBalancer,
	DeleteLoadBalancer,
	ValidateBackend,
	GenerateBackendAddr,
	EnsureBackend,
	DeregBackend,
	UpdateBackend,
)

// RequestForRetryHooks is the common request for webhooks that can be retried, including:
//
// createLoadBalancer, ensureLoadBalancer, deleteLoadBalancer, generateBackendAddr, ensureBackend, deregisterBackend
type RequestForRetryHooks struct {
	RecordID string `json:"recordID"`
	RetryID  string `json:"retryID"`
}

// ResponseForFailRetryHooks is the common response for webhooks that can be retried, including:
//
// createLoadBalancer, ensureLoadBalancer, deleteLoadBalancer, generateBackendAddr, ensureBackend, deregisterBackend
type ResponseForFailRetryHooks struct {
	Status                 string `json:"status"`
	Msg                    string `json:"msg"`
	MinRetryDelayInSeconds int32  `json:"minRetryDelayInSeconds"`
}

// ResponseForNoRetryHooks is the common response for webhooks that can NOT be retried, including:
//
// validateLoadBalancer, validateBackend
type ResponseForNoRetryHooks struct {
	Succ bool   `json:"succ"`
	Msg  string `json:"msg"`
}

const (
	// StatusSucc indicates webhook succeeded
	StatusSucc = "Succ"

	// StatusFail indicates webhook failed
	StatusFail = "Fail"

	// StatusRunning indicates webhook is still running
	StatusRunning = "Running"
)

// OperationType is used to distinguish why a webhook is called
type OperationType string

const (
	// OperationCreate indicates the webhook is called for an object is created in K8S
	OperationCreate OperationType = "Create"

	// OperationUpdate indicates the webhook is called for an object is updated in K8S
	OperationUpdate OperationType = "Update"
)

// ValidateLoadBalancerRequest is the request for webhook validateLoadBalancer
type ValidateLoadBalancerRequest struct {
	LBSpec        map[string]string `json:"lbSpec"`
	Operation     OperationType     `json:"operation"`
	Attributes    map[string]string `json:"attributes"`
	OldAttributes map[string]string `json:"oldAttributes,omitempty"`
}

// ValidateLoadBalancerResponse is the response for webhook validateLoadBalancer
type ValidateLoadBalancerResponse struct {
	ResponseForNoRetryHooks
}

// CreateLoadBalancerRequest is the request for webhook createLoadBalancer
type CreateLoadBalancerRequest struct {
	RequestForRetryHooks
	LBSpec     map[string]string `json:"lbSpec"`
	Attributes map[string]string `json:"attributes"`
}

// CreateLoadBalancerResponse is the response for webhook createLoadBalancer
type CreateLoadBalancerResponse struct {
	ResponseForFailRetryHooks
	LBInfo map[string]string `json:"lbInfo"`
}

// EnsureLoadBalancerRequest is the request for webhook ensureLoadBalancer
type EnsureLoadBalancerRequest struct {
	RequestForRetryHooks
	LBInfo     map[string]string `json:"lbInfo"`
	Attributes map[string]string `json:"attributes"`
}

// EnsureLoadBalancerResponse is the response for webhook ensureLoadBalancer
type EnsureLoadBalancerResponse struct {
	ResponseForFailRetryHooks
}

// DeleteLoadBalancerRequest is the request for webhook deleteLoadBalancer
type DeleteLoadBalancerRequest struct {
	RequestForRetryHooks
	LBInfo     map[string]string `json:"lbInfo"`
	Attributes map[string]string `json:"attributes"`
}

// DeleteLoadBalancerResponse is the response for webhook deleteLoadBalancer
type DeleteLoadBalancerResponse struct {
	ResponseForFailRetryHooks
}

// ValidateBackendRequest is the request for webhook validateBackend
type ValidateBackendRequest struct {
	BackendType   string            `json:"backendType"`
	LBInfo        map[string]string `json:"lbInfo"`
	Operation     OperationType     `json:"operation"`
	Parameters    map[string]string `json:"parameters"`
	OldParameters map[string]string `json:"OldParameters,omitempty"`
}

// ValidateBackendResponse is the response for webhook validateBackend
type ValidateBackendResponse struct {
	ResponseForNoRetryHooks
}

// GenerateBackendAddrRequest is the request for webhook generateBackendAddr
type GenerateBackendAddrRequest struct {
	RequestForRetryHooks
	LBInfo         map[string]string                    `json:"lbInfo"`
	LBAttributes   map[string]string                    `json:"lbAttributes"`
	Parameters     map[string]string                    `json:"parameters"`
	PodBackend     *PodBackendInGenerateAddrRequest     `json:"podBackend"`
	ServiceBackend *ServiceBackendInGenerateAddrRequest `json:"serviceBackend"`
}

// PodBackendInGenerateAddrRequest is part of GenerateBackendAddrRequest
type PodBackendInGenerateAddrRequest struct {
	Pod  v1.Pod               `json:"pod"`
	Port v1beta1.PortSelector `json:"port"`
}

// ServiceBackendInGenerateAddrRequest is part of GenerateBackendAddrRequest
type ServiceBackendInGenerateAddrRequest struct {
	Service v1.Service     `json:"service"`
	Port    v1.ServicePort `json:"port"`
	Node    v1.Node        `json:"node"`
}

// GenerateBackendAddrResponse is the response for webhook generateBackendAddr
type GenerateBackendAddrResponse struct {
	ResponseForFailRetryHooks
	BackendAddr string `json:"backendAddr"`
}

// BackendOperationRequest is the request for webhook ensureBackend and deregisterBackend
type BackendOperationRequest struct {
	RequestForRetryHooks
	LBInfo       map[string]string `json:"lbInfo"`
	BackendAddr  string            `json:"backendAddr"`
	Parameters   map[string]string `json:"parameters"`
	InjectedInfo map[string]string `json:"injectedInfo"`
}

// BackendOperationResponse is the response for webhook ensureBackend and deregisterBackend
type BackendOperationResponse struct {
	ResponseForFailRetryHooks
	InjectedInfo map[string]string `json:"injectedInfo"`
}
