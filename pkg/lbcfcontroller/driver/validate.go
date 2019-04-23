package driver

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"

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

func validateDriverWebhooks(raw *v1beta1.LoadBalancerDriverWebhook, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if raw.ValidateBackend != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.ValidateLoadBalancer, path.Child("validateLoadBalancer"))...)
	}

	if raw.CreateLoadBalancer != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.CreateLoadBalancer, path.Child("createLoadBalancer"))...)
	}

	if raw.UpdateLoadBalancer != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.UpdateLoadBalancer, path.Child("updateLoadBalancer"))...)
	}

	if raw.DeleteLoadBalancer != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.DeleteLoadBalancer, path.Child("deleteLoadBalancer"))...)
	}

	if raw.ValidateBackend != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.ValidateBackend, path.Child("validateBackend"))...)
	}

	if raw.GenerateBackendAddr != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.GenerateBackendAddr, path.Child("generateBackendAddr"))...)
	}

	if raw.EnsureBackend != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.EnsureBackend, path.Child("ensureBackend"))...)
	}

	if raw.DeregisterBackend != nil {
		allErrs = append(allErrs, validateDriverWebhookConfig(raw.DeregisterBackend, path.Child("deregisterBackend"))...)
	}
	return allErrs
}

func validateDriverWebhookConfig(raw *v1beta1.WebhookConfig, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw.Timeout == nil {
		return allErrs
	}
	if raw.Timeout.Nanoseconds() < (5*time.Second).Nanoseconds() || raw.Timeout.Nanoseconds() > (10*time.Minute).Nanoseconds() {
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
