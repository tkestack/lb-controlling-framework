package v1beta1

import (
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancer is a top-level type. A client is created for it.
type LoadBalancer struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec LoadBalancerSpec
	Status LoadBalancerStatus
}

type LoadBalancerSpec struct {
	LBType string
	LBSpec map[string]string
	attributes map[string]string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancerList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type LoadBalancerList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []LoadBalancer
}

type LoadBalancerStatus struct {
	LBInfo map[string]string
	Conditions []apiextensions.CustomResourceDefinitionCondition
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackendGroup is a top-level type. A client is created for it.
type BackendGroup struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec BackendGroupSpec
	Status BackendGroupStatus
}

type BackendGroupSpec struct {
	LBName string
	Service *ServiceBackend
	Pods *PodBackend
	Static *[]string
}

type ServiceBackend struct {
	Name string
	PortName *string
	NodeSelector map[string]string
}

type PodBackend struct {
	Port ContainerPort
	ByLabel *SelectPodByLabel
	ByName *[]string
}

type ContainerPort struct {
	PortNumber int32
	Protocol *string
}

type SelectPodByLabel struct {
	Selector map[string]string
	Except *[]string
}

type BackendGroupStatus struct {
	Backends int32
	RegisteredBackends int32
	Conditions []apiextensions.CustomResourceDefinitionCondition
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackendGroupList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type BackendGroupList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []BackendGroup
}
