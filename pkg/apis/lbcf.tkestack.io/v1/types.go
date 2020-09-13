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

package v1

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	APIVersion = "lbcf.tkestack.io/v1"

	SystemDriverPrefix = "lbcf-"

	// labels of LoadBalancerDriver
	DriverDrainingLabel = "lbcf.tkestack.io/driver-draining"
	LabelDriverName     = "lbcf.tkestack.io/driver"
	LabelLBName         = "lbcf.tkestack.io/lb"
	LabelBindName       = "lbcf.tkestack.io/bind"
	LabelPodName        = "lbcf.tkestack.io/pod"

	// LoadBalancers and BackendGroups with label do-not-delete are not allowed to be deleted
	LabelDoNotDelete = "lbcf.tkestack.io/do-not-delete"

	FinalizerDeleteLB          = "lbcf.tkestack.io/delete-load-loadbalancer"
	FinalizerDeregisterBackend = "lbcf.tkestack.io/deregister-backend"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Bind is a top-level type. A client is created for it.
type Bind struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BindSpec   `json:"spec"`
	Status BindStatus `json:"status,omitempty"`
}

type DeregPolicy string

const (
	DeregisterIfNotRunning DeregPolicy = "IfNotRunning"
	DeregisterIfNotReady   DeregPolicy = "IfNotReady"
	DeregisterWebhook      DeregPolicy = "Webhook"
)

type BindSpec struct {
	LoadBalancers []TargetLoadBalancer `json:"loadBalancers"`
	Pods          PodBackend           `json:"pods"`
	Parameters    map[string]string    `json:"parameters"`
	// +optional
	DeregisterPolicy *DeregPolicy `json:"deregisterPolicy"`
	// +optional
	DeregisterWebhook *DeregisterWebhookSpec `json:"deregisterWebhook"`
	// +optional
	EnsurePolicy *EnsurePolicyConfig `json:"ensurePolicy,omitempty"`
}

type TargetLoadBalancer struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Spec       map[string]string `json:"spec"`
	Attributes map[string]string `json:"attributes"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BindList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type BindList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Bind `json:"items"`
}

type BindStatus struct {
	LoadBalancerStatuses []TargetLoadBalancerStatus `json:"loadBalancerStatuses"`
}

type TargetLoadBalancerStatus struct {
	Name                 string            `json:"name"`
	Driver               string            `json:"driver"`
	LBInfo               map[string]string `json:"lbInfo,omitempty"`
	LastSyncedAttributes map[string]string `json:"lastSyncedAttributes,omitempty"`
	DeletionTimestamp    *string           `json:"deletionTimestamp"`
	// Retry after if Status is not True
	RetryAfter metav1.Time                   `json:"retryAfter,omitempty"`
	Conditions []TargetLoadBalancerCondition `json:"conditions,omitempty"`
}

type TargetLoadBalancerCondition struct {
	// Type is the type of the condition.
	Type LoadBalancerConditionType `json:"type"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	Reason ConditionReason `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
}

type LoadBalancerConditionType string

const (
	LBCreated LoadBalancerConditionType = "Created"
	LBReady   LoadBalancerConditionType = "Ready"
)

type deregFailurePolicy string

const (
	FailurePolicyDoNothing    deregFailurePolicy = "DoNothing"
	FailurePolicyIfNotReady   deregFailurePolicy = "IfNotReady"
	FailurePolicyIfNotRunning deregFailurePolicy = "IfNotRunning"
)

type DeregisterWebhookSpec struct {
	DriverName string `json:"driverName"`
	// +optional
	FailurePolicy *deregFailurePolicy `json:"failurePolicy"`
}

type PodBackend struct {
	Ports []PortSelector `json:"ports"`
	// +optional
	ByLabel *SelectPodByLabel `json:"byLabel,omitempty"`
	// +optional
	ByName []string `json:"byName,omitempty"`
}

type PortSelector struct {
	Port     int32  `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type SelectPodByLabel struct {
	Selector map[string]string `json:"selector"`
	// +optional
	Except []string `json:"except,omitempty"`
}

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var str string
	err := json.Unmarshal(b, &str)
	if err != nil {
		return err
	}

	dur, err := time.ParseDuration(str)
	if err != nil {
		return fmt.Errorf("invalid duration: %v, err: %v", str, err)
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

type ConditionReason string

const (
	ReasonCreating        ConditionReason = "Creating"
	ReasonCreateFailed    ConditionReason = "CreateFailed"
	ReasonEnsuring        ConditionReason = "Ensuring"
	ReasonEnsureFailed    ConditionReason = "EnsureFailed"
	ReasonWebhookError    ConditionReason = "WebhookError"
	ReasonInvalidResponse ConditionReason = "InvalidResponse"
)

func (c ConditionReason) String() string {
	return string(c)
}

type EnsurePolicyType string

const (
	PolicyIfNotSucc EnsurePolicyType = "IfNotSucc"
	PolicyAlways    EnsurePolicyType = "Always"
)

type EnsurePolicyConfig struct {
	Policy EnsurePolicyType `json:"policy"`
	// +optional
	ResyncPeriodInSeconds *int32 `json:"resyncPeriodInSeconds,omitempty"`
}
