package lbcfcontroller

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

type LBDriverType string

const (
	LBDriverWebhook LBDriverType = "Webhook"
)

const (
	DefaultWebhookTimeout = 10 * time.Second
	SystemDriverPrefix    = "lbcf-"
)

func ValidateLoadBalancerDriver(raw *v1beta1.LoadBalancerDriver) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateDriverName(raw.Name, raw.Namespace, field.NewPath("metadata").Child("name"))...)
	allErrs = append(allErrs, validateDriverType(raw.Spec.DriverType, field.NewPath("spec").Child("driverType"))...)
	//allErrs = append(allErrs, validateDriverUrl(raw.Spec.Url, field.NewPath("spec").Child("url"))...)
	if raw.Spec.Webhooks != nil {
		allErrs = append(allErrs, validateDriverWebhooks(raw.Spec.Webhooks, field.NewPath("spec").Child("webhooks"))...)
	}
	return allErrs
}

func validateDriverName(name string, namespace string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if namespace == "kube-system" {
		if !strings.HasPrefix(name, SystemDriverPrefix) {
			allErrs = append(allErrs, field.Invalid(path, name, fmt.Sprintf("metadata.name must start with %q for drivers in namespace %q", SystemDriverPrefix, "kube-system")))
		}
		return allErrs
	}
	if strings.HasPrefix(name, SystemDriverPrefix) {
		allErrs = append(allErrs, field.Invalid(path, name, fmt.Sprintf("metaname.name must not start with %q for drivers not in namespace %q", SystemDriverPrefix, "kube-system")))
	}
	return allErrs
}

func validateDriverType(raw string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw != string(LBDriverWebhook) {
		allErrs = append(allErrs, field.Invalid(path, raw, fmt.Sprintf("driverType must be %v", LBDriverWebhook)))

	}
	return allErrs
}

func validateDriverUrl(raw string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if _, err := url.Parse(raw); err != nil {
		allErrs = append(allErrs, field.Invalid(path, raw, err.Error()))
	}
	return allErrs
}

func validateDriverWebhooks(raw []v1beta1.WebhookConfig, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for _, wh := range raw {
		if !knownWebhooks.Has(wh.Name) {
			allErrs = append(allErrs, field.NotSupported(path, wh, knownWebhooks.List()))
		}
		if wh.Timeout != nil {
			if wh.Timeout.Nanoseconds() < (10*time.Second).Nanoseconds() || wh.Timeout.Nanoseconds() > (10*time.Minute).Nanoseconds() {
				allErrs = append(allErrs, field.Invalid(path, *wh.Timeout, fmt.Sprintf("webhook %s invalid, timeout of must be >= 10s and <= 10m", wh.Name)))
				return allErrs
			}
		}

	}
	return allErrs
}

func validateDriverWebhookConfig(raw *v1beta1.WebhookConfig, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw.Timeout == nil {
		return allErrs
	}
	if raw.Timeout.Nanoseconds() < (10*time.Second).Nanoseconds() || raw.Timeout.Nanoseconds() > (10*time.Minute).Nanoseconds() {
		allErrs = append(allErrs, field.Invalid(path, *raw.Timeout, "timeout must be >= 5s and <= 10m"))
		return allErrs
	}
	return allErrs
}

func DriverUpdatedFieldsAllowed(cur *v1beta1.LoadBalancerDriver, old *v1beta1.LoadBalancerDriver) bool {
	if cur.Name != old.Name {
		return false
	}
	if !reflect.DeepEqual(cur.Spec, old.Spec) {
		return false
	}
	return true
}

func LBUpdatedFieldsAllowed(cur *v1beta1.LoadBalancer, old *v1beta1.LoadBalancer) bool {
	if cur.Name != old.Name {
		return false
	}
	if cur.Spec.LBDriver != old.Spec.LBDriver {
		return false
	}
	if !reflect.DeepEqual(cur.Spec.LBSpec, old.Spec.LBSpec) {
		return false
	}
	return true
}

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
		} else {
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

func validatePodBackend(raw *v1beta1.PodBackend, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validatePortSelector(raw.Port, path.Child("port"))...)
	if raw.ByLabel != nil {
		if raw.ByName != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("byName"), raw.ByName, "only one of \"byLabel, byName\" is allowed"))
		}
	}

	if raw.ByName == nil {
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

func BackendGroupUpdateFieldsAllowed(cur *v1beta1.BackendGroup, old *v1beta1.BackendGroup) bool {
	if getBackendType(cur) != getBackendType(old) {
		return false
	}
	return true
}
