package v1beta1

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
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
	LBCreated LoadBalancerConditionType = "Created"
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
	Conditions         []apiextensions.CustomResourceDefinitionCondition `json:"conditions"`
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
	LBName            string            `json:"lbName"`
	LBInfo            map[string]string `json:"lbInfo"`
	LBAttributes      map[string]string `json:"lbAttributes"`
	BackendParameters map[string]string `json:"backendParameters"`
	DriverInjection   map[string]string `json:"driverInjection"`
}

type BackendRecordStatus struct {
	BackendAddr string                                            `json:"backendAddr"`
	Conditions  []apiextensions.CustomResourceDefinitionCondition `json:"conditions"`
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
