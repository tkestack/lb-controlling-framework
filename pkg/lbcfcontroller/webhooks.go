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
	RetryIntervalInSeconds int32  `json:"retryIntervalInSeconds"`
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
	Port v1.ContainerPort `json:"port"`
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

type EnsureBackendRequest struct {
	LBInfo   map[string]string `json:"lbInfo"`
	Backends []BackendReg      `json:"backends"`
}

type BackendReg struct {
	RequestForRetryHooks
	BackendAddr  string            `json:"backendAddr"`
	Parameters   map[string]string `json:"parameters"`
	InjectedInfo map[string]string `json:"injectedInfo"`
}

type EnsureBackendResponse struct {
	TargetSpec map[string]string  `json:"targetSpec"`
	Results    []BackendRegResult `json:"results"`
}

type BackendRegResult struct {
	RecordID string `json:"recordID"`
	RetryID  string `json:"retryID"`
	ResponseForFailRetryHooks
	Ingress      []v1.LoadBalancerIngress `json:"ingress"`
	InjectedInfo map[string]string        `json:"injectedInfo"`
}

type DeregisterBackendRequest struct {
	LBInfo   map[string]string `json:"lbInfo"`
	Backends []BackendDereg    `json:"backends"`
}

type BackendDereg struct {
	RequestForRetryHooks
	BackendAddr  string            `json:"backendAddr"`
	Parameters   map[string]string `json:"parameters"`
	InjectedInfo map[string]string `json:"injectedInfo"`
}

type DeregisterBackendResponse struct {
	LBInfo  map[string]string    `json:"lbInfo"`
	Results []BackendDeregResult `json:"results"`
}

type BackendDeregResult struct {
	RecordID string `json:"recordID"`
	RetryID  string `json:"retryID"`
	ResponseForFailRetryHooks
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

//func callValidateLoadBalancer(driver *v1beta1.LoadBalancerDriver, req *ValidateLoadBalancerRequest) (*ValidateLoadBalancerResponse, error) {
//	rsp := &ValidateLoadBalancerResponse{}
//	if err := callWebhook(driver, ValidateBEHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callCreateLoadBalancer(driver *v1beta1.LoadBalancerDriver, req *CreateLoadBalancerRequest) (*CreateLoadBalancerResponse, error) {
//	rsp := &CreateLoadBalancerResponse{}
//	if err := callWebhook(driver, CreateLBHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callEnsureLoadBalancer(driver *v1beta1.LoadBalancerDriver, req *EnsureLoadBalancerRequest) (*EnsureLoadBalancerResponse, error) {
//	rsp := &EnsureLoadBalancerResponse{}
//	if err := callWebhook(driver, EnsureLBHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callDeleteLoadBalancer(driver *v1beta1.LoadBalancerDriver, req *DeleteLoadBalancerRequest) (*DeleteLoadBalancerResponse, error) {
//	rsp := &DeleteLoadBalancerResponse{}
//	if err := callWebhook(driver, DeleteLBHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callValidateBackend(driver *v1beta1.LoadBalancerDriver, req *ValidateBackendRequest) (*ValidateBackendResponse, error) {
//	rsp := &ValidateBackendResponse{}
//	if err := callWebhook(driver, ValidateBEHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callGenerateBackendAddr(driver *v1beta1.LoadBalancerDriver, req *GenerateBackendAddrRequest) (*GenerateBackendAddrResponse, error) {
//	rsp := &GenerateBackendAddrResponse{}
//	if err := callWebhook(driver, GenerateBEAddrHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callEnsureBackend(driver *v1beta1.LoadBalancerDriver, req *EnsureBackendRequest) (*EnsureBackendResponse, error) {
//	rsp := &EnsureBackendResponse{}
//	if err := callWebhook(driver, EnsureBEHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
//
//func callDeregisterBackend(driver *v1beta1.LoadBalancerDriver, req *DeregisterBackendRequest) (*DeregisterBackendResponse, error) {
//	rsp := &DeregisterBackendResponse{}
//	if err := callWebhook(driver, DeregBEHook, req, rsp); err != nil {
//		return nil, err
//	}
//	return rsp, nil
//}
