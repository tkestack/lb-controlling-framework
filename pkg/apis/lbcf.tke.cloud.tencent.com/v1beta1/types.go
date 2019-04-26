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

package v1beta1

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancer is a top-level type. A client is created for it.
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

type LoadBalancerSpec struct {
	LBDriver string            `json:"lbDriver"`
	LBSpec   map[string]string `json:"lbSpec"`
	// +optional
	Attributes map[string]string `json:"attributes"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancerList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type LoadBalancerList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Items []LoadBalancer `json:"items"`
}

type LoadBalancerStatus struct {
	LBInfo     map[string]string       `json:"lbInfo"`
	Conditions []LoadBalancerCondition `json:"conditions"`
}

type LoadBalancerCondition struct {
	// Type is the type of the condition.
	Type LoadBalancerConditionType
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status ConditionStatus
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string
	// Human-readable message indicating details about last transition.
	// +optional
	Message string
}

type LoadBalancerConditionType string

const (
	LBCreated   LoadBalancerConditionType = "Created"
	LBSatisfied LoadBalancerConditionType = "Satisfied"
	LBDeleted  LoadBalancerConditionType = "Deleted"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackendGroup is a top-level type. A client is created for it.
type BackendGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackendGroupSpec   `json:"spec"`
	Status BackendGroupStatus `json:"status,omitempty"`
}

type BackendGroupSpec struct {
	LBName string `json:"lbName"`
	// +optional
	Service *ServiceBackend `json:"service,omitempty"`
	// +optional
	Pods *PodBackend `json:"pods,omitempty"`
	// +optional
	Static []string `json:"static,omitempty"`
	// +optional
	Parameters map[string]string `json:"parameters"`
}

type ServiceBackend struct {
	Name string       `json:"name"`
	Port PortSelector `json:"port,omitempty"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

type PodBackend struct {
	Port PortSelector `json:"port"`
	// +optional
	ByLabel *SelectPodByLabel `json:"byLabel,omitempty"`
	// +optional
	ByName []string `json:"byName,omitempty"`
}

type PortSelector struct {
	PortNumber int32 `json:"portNumber"`
	// +optional
	Protocol *string `json:"protocol,omitempty"`
}

type SelectPodByLabel struct {
	Selector map[string]string `json:"selector"`
	// +optional
	Except []string `json:"except,omitempty"`
}

type BackendGroupStatus struct {
	Backends           int32                                             `json:"backends"`
	RegisteredBackends int32                                             `json:"registeredBackends"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackendGroupList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type BackendGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []BackendGroup `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancerDriver is a top-level type. A client is created for it.
type LoadBalancerDriver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerDriverSpec   `json:"spec"`
	Status LoadBalancerDriverStatus `json:"status,omitempty"`
}

type LoadBalancerDriverSpec struct {
	DriverType string `json:"driverType"`
	Url        URL    `json:"url"`
	// +optional
	Webhooks []WebhookConfig `json:"webhooks"`
}

type WebhookConfig struct {
	Name    string    `json:"name"`
	Timeout *Duration `json:"timeout"`
}

type LoadBalancerDriverConditionType string

const (
	DriverAccepted LoadBalancerDriverConditionType = "Accepted"
)

type LoadBalancerDriverCondition struct {
	// Type is the type of the condition.
	Type LoadBalancerDriverConditionType
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status ConditionStatus
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string
	// Human-readable message indicating details about last transition.
	// +optional
	Message string
}

type LoadBalancerDriverStatus struct {
	Conditions []LoadBalancerDriverCondition `json:"conditions"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancerDriverList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type LoadBalancerDriverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []LoadBalancerDriver `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancerDriver is a top-level type. A client is created for it.
type BackendRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackendRecordSpec   `json:"spec"`
	Status BackendRecordStatus `json:"status,omitempty"`
}

type BackendRecordSpec struct {
	LBName             string                `json:"lbName"`
	LBDriver           string                `json:"lbDriver"`
	LBInfo             map[string]string     `json:"lbInfo"`
	LBAttributes       map[string]string     `json:"lbAttributes"`
	Parameters         map[string]string     `json:"parameters"`
	PodBackendInfo     *PodBackendRecord     `json:"podBackend"`
	ServiceBackendInfo *ServiceBackendRecord `json:"serviceBackend"`
}

type PodBackendRecord struct {
	Name string        `json:"name"`
	Port ContainerPort `json:"port"`
}

type ServiceBackendRecord struct {
	Name string            `json:"name"`
	Port ServicePort       `json:"port"`
	Node NodeBackendRecord `json:"node"`
}

type NodeBackendRecord struct {
	Name string `json:"name"`
}

type ContainerPort struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	HostPort      int32 `json:"hostPort,omitempty"`
	ContainerPort int32 `json:"containerPort"`
	// +optional
	Protocol string `json:"protocol,omitempty"`
	// +optional
	HostIP string `json:"hostIP,omitempty"`
}

type ServicePort struct {
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Protocol string `json:"protocol,omitempty"`

	// The port that will be exposed by this service.
	Port int32 `json:"port" protobuf:"varint,3,opt,name=port"`

	// +optional
	TargetPort IntOrString `json:"targetPort,omitempty" protobuf:"bytes,4,opt,name=targetPort"`

	// The port on each node on which this service is exposed when type=NodePort or LoadBalancer.
	// Usually assigned by the system. If specified, it will be allocated to the service
	// if unused or else creation of the service will fail.
	// Default is to auto-allocate a port if the ServiceType of this Service requires one.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#type-nodeport
	// +optional
	NodePort int32 `json:"nodePort,omitempty" protobuf:"varint,5,opt,name=nodePort"`
}

type Type int

const (
	Int    Type = iota // The IntOrString holds an int.
	String             // The IntOrString holds a string.
)

type IntOrString struct {
	Type   Type   `protobuf:"varint,1,opt,name=type,casttype=Type"`
	IntVal int32  `protobuf:"varint,2,opt,name=intVal"`
	StrVal string `protobuf:"bytes,3,opt,name=strVal"`
}

func FromInt(val int) IntOrString {
	if val > math.MaxInt32 || val < math.MinInt32 {
		//glog.Errorf("value: %d overflows int32\n%s\n", val, debug.Stack())
	}
	return IntOrString{Type: Int, IntVal: int32(val)}
}

// FromString creates an IntOrString object with a string value.
func FromString(val string) IntOrString {
	return IntOrString{Type: String, StrVal: val}
}

// Parse the given string and try to convert it to an integer before
// setting it as a string value.
func Parse(val string) IntOrString {
	i, err := strconv.Atoi(val)
	if err != nil {
		return FromString(val)
	}
	return FromInt(i)
}

// String returns the string value, or the Itoa of the int value.
func (intstr *IntOrString) String() string {
	if intstr.Type == String {
		return intstr.StrVal
	}
	return strconv.Itoa(intstr.IntValue())
}

// IntValue returns the IntVal if type Int, or if
// it is a String, will attempt a conversion to int.
func (intstr *IntOrString) IntValue() int {
	if intstr.Type == String {
		i, _ := strconv.Atoi(intstr.StrVal)
		return i
	}
	return int(intstr.IntVal)
}

func (intstr *IntOrString) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		intstr.Type = String
		return json.Unmarshal(value, &intstr.StrVal)
	}
	intstr.Type = Int
	return json.Unmarshal(value, &intstr.IntVal)
}

func (intstr IntOrString) MarshalJSON() ([]byte, error) {
	switch intstr.Type {
	case Int:
		return json.Marshal(intstr.IntVal)
	case String:
		return json.Marshal(intstr.StrVal)
	default:
		return []byte{}, fmt.Errorf("impossible IntOrString.Type")
	}
}

type BackendRecordStatus struct {
	BackendAddr  string                   `json:"backendAddr"`
	InjectedInfo map[string]string        `json:"injectedInfo"`
	Conditions   []BackendRecordCondition `json:"conditions"`
}

type BackendRecordConditionType string

const (
	BackendRegistered BackendRecordConditionType = "Registered"
)

type BackendRecordCondition struct {
	// Type is the type of the condition.
	Type BackendRecordConditionType
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status ConditionStatus
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string
	// Human-readable message indicating details about last transition.
	// +optional
	Message string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackendRecordList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type BackendRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []BackendRecord `json:"items"`
}

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

type URL struct {
	url.URL
}

func (u *URL) UnmarshalJSON(b []byte) error {
	var str string
	err := json.Unmarshal(b, &str)
	if err != nil {
		return err
	}

	parsed, err := url.Parse(str)
	if err != nil {
		return fmt.Errorf("invalid URL: %v, err: %v", str, err)
	}
	u.URL = *parsed
	return nil
}

func (u URL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.URL.String())
}

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

const(
	ReasonOperationInProgress ConditionReason = "InProgres"
	ReasonOperationFailed ConditionReason = "OperationFailed"
	ReasonInvalidResponse ConditionReason = "InvalidResponse"
)

func (c ConditionReason) String() string{
	return string(c)
}