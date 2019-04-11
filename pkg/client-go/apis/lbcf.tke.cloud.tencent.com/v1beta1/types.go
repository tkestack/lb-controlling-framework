package v1beta1

import (
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
	LBType     string            `json:"lbType"`
	LBSpec     map[string]string `json:"lbSpec"`
	attributes map[string]string `json:"attributes"`
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
	LBInfo     map[string]string                                 `json:"lbInfo"`
	Conditions []apiextensions.CustomResourceDefinitionCondition `json:"conditions"`
}

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
	LBName  string          `json:"lbName"`
	Service *ServiceBackend `json:"service,omitempty"`
	Pods    *PodBackend     `json:"pods,omitempty"`
	Static  *[]string       `json:"static,omitempty"`
}

type ServiceBackend struct {
	Name         string            `json:"name"`
	PortName     *string           `json:"portName,omitempty"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

type PodBackend struct {
	Port    ContainerPort     `json:"port"`
	ByLabel *SelectPodByLabel `json:"byLabel,omitempty"`
	ByName  *[]string         `json:"byName,omitempty"`
}

type ContainerPort struct {
	PortNumber int32   `json:"portNumber"`
	Protocol   *string `json:"protocol,omitempty"`
}

type SelectPodByLabel struct {
	Selector map[string]string `json:"selector"`
	Except   *[]string         `json:"except,omitempty"`
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
	DriverType string                     `json:"driverType"`
	Addr       string                     `json:"addr"`
	Webhooks   *LoadBalancerDriverWebhook `json:"webhooks"`
}

type LoadBalancerDriverWebhook struct {
	ValidateLoadBalancer *WebhookConfig `json:"validateLoadBalancer,omitempty"`
	CreateLoadBalancer   *WebhookConfig `json:"createLoadBalancer,omitempty"`
	UpdateLoadBalancer   *WebhookConfig `json:"updateLoadBalancer,omitempty"`
	DeleteLoadBalancer   *WebhookConfig `json:"deleteLoadBalancer,omitempty"`
	ValidateBackend      *WebhookConfig `json:"validateBackend,omitempty"`
	GenerateBackendAddr  *WebhookConfig `json:"generateBackendAddr,omitempty"`
	EnsureBackend        *WebhookConfig `json:"ensureBackend,omitempty"`
	DeregisterBackend    *WebhookConfig `json:"deregisterBackend,omitempty"`
	UpdateBackend        *WebhookConfig `json:"updateBackend,omitempty"`
}

type WebhookConfig struct {
	Timeout *string `json:"timeout"`
}

type LoadBalancerDriverStatus struct {
	Conditions []apiextensions.CustomResourceDefinitionCondition `json:"conditions"`
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
