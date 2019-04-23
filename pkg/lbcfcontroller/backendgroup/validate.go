package backendgroup

import (
	"strings"

	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func ValidateBackendGroup(raw *v1beta1.BackendGroup) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateBackends(&raw.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateBackends(raw *v1beta1.BackendGroupSpec, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if raw.Service != nil {
		if raw.Pods != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("pods"), raw.Pods, "only one of \"service, pods, static\" is allowed"))
		} else if raw.Static != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("static"), raw.Pods, "only one of \"service, pods, static\" is allowed"))
		} else {
			allErrs = append(allErrs, validateServiceBackend(raw.Service, path.Child("service"))...)
		}
		return allErrs
	}

	if raw.Pods != nil {
		if raw.Static != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("static"), raw.Pods, "only one of \"service, pods, static\" is allowed"))
		}else{
			allErrs = append(allErrs, validatePodBackend(raw.Pods, path.Child("pods"))...)
		}
		return allErrs
	}

	if raw.Static == nil {
		allErrs = append(allErrs, field.Required(path.Child("service/pods/static"), "one of \"service, pods, static\" must be specified"))
	}
	return allErrs
}

func validateServiceBackend(raw *v1beta1.ServiceBackend, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validatePortSelector(raw.Port, path.Child("port"))...)
	return allErrs
}

func validatePodBackend(raw *v1beta1.PodBackend, path *field.Path) field.ErrorList{
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validatePortSelector(raw.Port, path.Child("port"))...)
	if raw.ByLabel != nil{
		if raw.ByName != nil{
			allErrs = append(allErrs, field.Invalid(path.Child("byName"), raw.ByName, "only one of \"byLabel, byName\" is allowed"))
		}
	}

	if raw.ByName == nil{
		allErrs = append(allErrs, field.Required(path.Child("byLabel/byName"), "one of \"byLabel, byName\" must be specified"))
	}
	return allErrs
}

func validatePortSelector(raw v1beta1.PortSelector, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if raw.PortNumber <= 0 || raw.PortNumber > 65535 {
		allErrs = append(allErrs, field.Invalid(path.Child("portNumber"), raw.PortNumber, "portNumber must be greater than 0 and less than 65536"))
	}

	if raw.Protocol != nil {
		p := strings.ToUpper(*raw.Protocol)
		if p != string(v1.ProtocolTCP) && p != string(v1.ProtocolUDP) {
			allErrs = append(allErrs, field.Invalid(path.Child("protocol"), raw.Protocol, "portNumber must be \"TCP\" or \"UDP\""))
		}
	}
	return allErrs
}

func FieldModificationAllowed(cur *v1beta1.BackendGroup, old *v1beta1.BackendGroup) bool{
	 if getBackendType(cur.Spec) != getBackendType(old.Spec){
	 	return false
	 }
	return true
}

func getBackendType(spec v1beta1.BackendGroupSpec) BackendType{
	if spec.Service != nil{
		return TypeService
	}else if spec.Pods != nil{
		return TypePod
	}
	return TypeStatic
}

