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

package admission

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	lbcfapiv1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateLoadBalancerDriver validates LoadBalancerDriver
func ValidateLoadBalancerDriver(raw *lbcfapi.LoadBalancerDriver) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateDriverName(raw.Name, raw.Namespace, field.NewPath("metadata").Child("name"))...)
	allErrs = append(allErrs, validateDriverType(raw.Spec.DriverType, field.NewPath("spec").Child("driverType"))...)
	allErrs = append(allErrs, validateDriverURL(raw.Spec.URL, field.NewPath("spec").Child("url"))...)
	allErrs = append(allErrs, validateDriverWebhooks(raw.Spec.Webhooks, field.NewPath("spec").Child("webhooks"))...)
	return allErrs
}

// ValidateLoadBalancer validates LoadBalancer
func ValidateLoadBalancer(raw *lbcfapi.LoadBalancer) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw.Spec.LBDriver == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("lbDriver"), "lbDriver must be specified"))
	}
	if strings.HasPrefix(raw.Name, "lbcf-") && raw.Namespace != "kube-system" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("name"), raw.Name,
			"LoadBalancer that has name begins with \"lbcf-\" must be create in namespace kube-system"))
	}
	if len(raw.Spec.Scope) > 0 {
		if !strings.HasPrefix(raw.Name, "lbcf-") {
			allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("name"), raw.Name,
				"name of LoadBalancer that has non-empty scope must begins with \"lbcf-\""))
		} else if raw.Namespace != "kube-system" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("namespace"), raw.Name,
				"namespace of LoadBalancer that has non-empty scope must be kube-system"))
		}
	}
	if raw.Spec.EnsurePolicy != nil {
		allErrs = append(allErrs, validateEnsurePolicy(*raw.Spec.EnsurePolicy, field.NewPath("spec").Child("ensurePolicy"))...)
	}
	return allErrs
}

// ValidateBackendGroup validates BackendGroup
func ValidateBackendGroup(raw *lbcfapi.BackendGroup) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateTargetLoadBalancer(raw, field.NewPath("spec"))...)
	if raw.Spec.EnsurePolicy != nil {
		allErrs = append(allErrs, validateEnsurePolicy(*raw.Spec.EnsurePolicy, field.NewPath("spec").Child("ensurePolicy"))...)
	}
	if raw.Spec.DeregisterPolicy != nil {
		allErrs = append(allErrs, validateDeregisterPolicy(*raw.Spec.DeregisterPolicy, field.NewPath("spec").Child("deregisterPolicy"))...)
		allErrs = append(allErrs, validateDeregWebhookSpec(raw.Spec.DeregisterWebhook, *raw.Spec.DeregisterPolicy, field.NewPath("spec").Child("deregisterWebhook"))...)
	}
	allErrs = append(allErrs, validateBackends(&raw.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateBind validates Bind
func ValidateBind(bind *lbcfapiv1.Bind) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(bind.Spec.LoadBalancers) == 0 {
		allErrs = append(
			allErrs,
			field.Required(field.NewPath("spec").Child("loadbalancers"), "loadbalancers must be set"))
		return allErrs
	}

	// validate load balancers
	lbNames := sets.NewString()
	for i, lb := range bind.Spec.LoadBalancers {
		if lbNames.Has(lb.Name) {
			allErrs = append(
				allErrs,
				field.Duplicate(field.NewPath("spec").Child(fmt.Sprintf("loadbalancers[%d]", i)), lb.Name))
			continue
		}
		lbNames.Insert(lb.Name)
		if strings.TrimSpace(lb.Driver) == "" {
			allErrs = append(allErrs,
				field.Required(
					field.NewPath("spec").
						Child(fmt.Sprintf("loadbalancers[%d]", i), "driver"),
					"driver must be set"))
			continue
		}
		if len(lb.Spec) == 0 {
			allErrs = append(allErrs,
				field.Required(
					field.NewPath("spec").
						Child(fmt.Sprintf("loadbalancers[%d]", i), "spec"),
					"spec must be set"))
			continue
		}
	}

	// validate pods
	if bind.Spec.Pods.ByLabel == nil && len(bind.Spec.Pods.ByName) == 0 {
		allErrs = append(allErrs,
			field.Required(
				field.NewPath("spec").
					Child("pods", "byLabel"),
				"one of byLabel and byName must be set"))
	}

	// validate pods.byLabel
	if bind.Spec.Pods.ByLabel != nil {
		if len(bind.Spec.Pods.ByLabel.Selector) == 0 {
			allErrs = append(allErrs,
				field.Required(
					field.NewPath("spec").
						Child("pods", "byLabel", "selector"),
					"selector must be set if byLabel is used"))
		}
	}

	// validate pods.ports
	if len(bind.Spec.Pods.Ports) == 0 {
		allErrs = append(allErrs,
			field.Required(
				field.NewPath("spec").
					Child("pods", "ports"),
				"ports must be set for pods"))
	}
	for i, port := range bind.Spec.Pods.Ports {
		if port.Port == 0 {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec").
						Child("pods", fmt.Sprintf("ports[%d]", i), "port"),
					port.Port,
					"port must not be 0"))
		}
		if strings.TrimSpace(port.Protocol) == "" {
			allErrs = append(allErrs,
				field.Required(
					field.NewPath("spec").
						Child("pods", fmt.Sprintf("ports[%d]", i), "protocol"),
					"protocol in port must be set, supported protocol: tcp, udp"))
		}
		protocol := strings.ToLower(port.Protocol)
		if protocol != "tcp" && protocol != "udp" {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec").
						Child("pods", fmt.Sprintf("ports[%d]", i), "protocol"),
					port.Protocol,
					fmt.Sprintf("supported protocol: tcp, udp")))
		}
	}

	// validate deregisterPolicy
	if bind.Spec.DeregisterPolicy != nil {
		availablePolicies := sets.NewString(
			string(lbcfapiv1.DeregisterIfNotReady),
			string(lbcfapiv1.DeregisterIfNotRunning),
			string(lbcfapiv1.DeregisterWebhook))
		if availablePolicies.Has(string(*bind.Spec.DeregisterPolicy)) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec").
						Child("deregisterPolicy"),
					*bind.Spec.DeregisterPolicy,
					fmt.Sprintf("supported policy: %s", strings.Join(availablePolicies.List(), ","))))
		}
	}

	// validate deregisterWebhook
	if bind.Spec.DeregisterPolicy != nil &&
		*bind.Spec.DeregisterPolicy == lbcfapiv1.DeregisterWebhook &&
		bind.Spec.DeregisterWebhook == nil {
		allErrs = append(allErrs,
			field.Required(
				field.NewPath("spec").
					Child("deregisterWebhook"),
				"deregisterWebhook must be set if deregisterPolicy is Webhook"))
	}
	if bind.Spec.DeregisterWebhook != nil {
		if bind.Spec.DeregisterWebhook.DriverName == "" {
			allErrs = append(allErrs,
				field.Required(
					field.NewPath("spec").
						Child("deregisterWebhook", "driverName"),
					"driverName must be set in deregisterWebhook"))
		}
		if bind.Spec.DeregisterWebhook.FailurePolicy != nil {
			availablePolicies := sets.NewString(
				string(lbcfapiv1.FailurePolicyDoNothing),
				string(lbcfapiv1.FailurePolicyIfNotReady),
				string(lbcfapiv1.FailurePolicyIfNotRunning))
			if !availablePolicies.Has(string(*bind.Spec.DeregisterWebhook.FailurePolicy)) {
				allErrs = append(allErrs,
					field.Invalid(
						field.NewPath("spec").
							Child("deregisterWebhook", "failurePolicy"),
						*bind.Spec.DeregisterWebhook.FailurePolicy,
						fmt.Sprintf("supported policy: %s", strings.Join(availablePolicies.List(), ","))))
			}
		}
	}

	// validate ensurePolicy
	if bind.Spec.EnsurePolicy != nil {
		if bind.Spec.EnsurePolicy.Policy == "" {
			allErrs = append(allErrs,
				field.Required(
					field.NewPath("spec").
						Child("ensurePolicy", "policy"),
					"policy must be set in ensurePolicy"))
		} else {
			availablePolicies := sets.NewString(
				string(lbcfapiv1.PolicyIfNotSucc),
				string(lbcfapiv1.PolicyAlways))
			if !availablePolicies.Has(string(bind.Spec.EnsurePolicy.Policy)) {
				allErrs = append(allErrs,
					field.Invalid(
						field.NewPath("spec").
							Child("ensurePolicy", "policy"),
						bind.Spec.EnsurePolicy.Policy,
						fmt.Sprintf("supported policy: %s", strings.Join(availablePolicies.List(), ","))))
			}
		}
		if bind.Spec.EnsurePolicy.ResyncPeriodInSeconds != nil && *bind.Spec.EnsurePolicy.ResyncPeriodInSeconds < 10 {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec").
						Child("ensurePolicy", "resyncPeriodInSeconds"),
					*bind.Spec.EnsurePolicy.ResyncPeriodInSeconds,
					"resyncPeriodInSeconds must be greater than or equal to 10"))
		}
	}
	return allErrs
}

// DriverUpdatedFieldsAllowed returns false if the updating to fields is not allowed
func DriverUpdatedFieldsAllowed(cur *lbcfapi.LoadBalancerDriver, old *lbcfapi.LoadBalancerDriver) (bool, string) {
	if old.Spec.URL != cur.Spec.URL {
		return false, "updating URL is prohibited"
	}
	if old.Spec.DriverType != cur.Spec.DriverType {
		return false, "updating driverType is prohibited"
	}
	return true, ""
}

// LBUpdatedFieldsAllowed returns false if the updating to fields is not allowed
func LBUpdatedFieldsAllowed(cur *lbcfapi.LoadBalancer, old *lbcfapi.LoadBalancer) (bool, string) {
	if cur.Spec.LBDriver != old.Spec.LBDriver {
		return false, "updating lbDriver is prohibited"
	}
	if !reflect.DeepEqual(cur.Spec.LBSpec, old.Spec.LBSpec) {
		return false, "updating lbSpec is prohibited"
	}
	return true, ""
}

// BackendGroupUpdateFieldsAllowed returns false if the updating to fields is not allowed
func BackendGroupUpdateFieldsAllowed(cur *lbcfapi.BackendGroup, old *lbcfapi.BackendGroup) (bool, string) {
	if util.GetBackendType(cur) != util.GetBackendType(old) {
		return false, "changing backend type is prohibited"
	}
	return true, ""
}

func validateEnsurePolicy(raw lbcfapi.EnsurePolicyConfig, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch raw.Policy {
	case lbcfapi.PolicyIfNotSucc:
		if raw.MinPeriod != nil {
			allErrs = append(allErrs, field.Forbidden(path.Child("minPeriod"), fmt.Sprintf("minPeriod is not supported when policy is %q", string(lbcfapi.PolicyIfNotSucc))))
		}
	case lbcfapi.PolicyAlways:
		if raw.MinPeriod != nil {
			if raw.MinPeriod.Nanoseconds() < 30*time.Second.Nanoseconds() {
				allErrs = append(allErrs, field.Invalid(path.Child("minPeriod"), raw.MinPeriod, "minPeriod must be greater or equal to 30s"))
			}
		}
	}
	return allErrs
}

func validateDeregisterPolicy(raw lbcfapi.DeregPolicy, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw != lbcfapi.DeregisterIfNotReady &&
		raw != lbcfapi.DeregisterIfNotRunning &&
		raw != lbcfapi.DeregisterWebhook {
		allErrs = append(allErrs, field.Invalid(path, raw, fmt.Sprintf("deregisterPolicy must be one of [%s, %s, %s]",
			lbcfapi.DeregisterIfNotReady, lbcfapi.DeregisterIfNotRunning, lbcfapi.DeregisterWebhook)))
	}
	return allErrs
}

func validateDeregWebhookSpec(raw *lbcfapi.DeregisterWebhookSpec, policy lbcfapi.DeregPolicy, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if policy != lbcfapi.DeregisterWebhook {
		return allErrs
	}
	if raw == nil {
		allErrs = append(allErrs, field.Required(path,
			fmt.Sprintf("deregisterWebhook must be specified when spec.deregisterPolicy is %s", lbcfapi.DeregisterWebhook)))
		return allErrs
	}
	if raw.DriverName == "" {
		allErrs = append(allErrs, field.Required(path.Child("driverName"), ""))
		return allErrs
	}
	if raw.FailurePolicy != nil &&
		*raw.FailurePolicy != lbcfapi.FailurePolicyDoNothing &&
		*raw.FailurePolicy != lbcfapi.FailurePolicyIfNotRunning &&
		*raw.FailurePolicy != lbcfapi.FailurePolicyIfNotReady {
		allErrs = append(allErrs,
			field.Invalid(path.Child("failurePolicy"),
				*raw.FailurePolicy,
				fmt.Sprintf("deregisterPolicy must be one of [%s, %s, %s]",
					lbcfapi.FailurePolicyDoNothing, lbcfapi.FailurePolicyIfNotRunning, lbcfapi.FailurePolicyIfNotReady)))
	}
	return allErrs
}

func validateDriverName(name string, namespace string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if namespace == metav1.NamespaceSystem {
		if !strings.HasPrefix(name, lbcfapi.SystemDriverPrefix) {
			allErrs = append(allErrs, field.Invalid(path, name, fmt.Sprintf("metadata.name must start with %q for drivers in namespace %q", lbcfapi.SystemDriverPrefix, metav1.NamespaceSystem)))
		}
		return allErrs
	}
	if strings.HasPrefix(name, lbcfapi.SystemDriverPrefix) {
		allErrs = append(allErrs, field.Invalid(path, name, fmt.Sprintf("metaname.name must not start with %q for drivers not in namespace %q", lbcfapi.SystemDriverPrefix, metav1.NamespaceSystem)))
	}
	return allErrs
}

func validateDriverType(raw string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw != string(lbcfapi.WebhookDriver) {
		allErrs = append(allErrs, field.Invalid(path, raw, fmt.Sprintf("driverType must be %v", lbcfapi.WebhookDriver)))
	}
	return allErrs
}

func validateDriverURL(raw string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if _, err := url.Parse(raw); err != nil {
		allErrs = append(allErrs, field.Invalid(path, raw, err.Error()))
	}
	return allErrs
}

func validateDriverWebhooks(raw []lbcfapi.WebhookConfig, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	var supported []string
	for known := range webhooks.KnownWebhooks {
		supported = append(supported, known)
	}

	hasWebhook := make(map[string]lbcfapi.WebhookConfig)
	for _, wh := range raw {
		hasWebhook[wh.Name] = wh
		if _, ok := webhooks.KnownWebhooks[wh.Name]; !ok {
			allErrs = append(allErrs, field.NotSupported(path.Child(wh.Name).Child("name"), wh.Name, supported))
		}
	}
	if len(allErrs) > 0 {
		return allErrs
	}

	for known := range webhooks.KnownWebhooks {
		wh, ok := hasWebhook[known]
		if !ok {
			allErrs = append(allErrs, field.Required(path.Child(known), fmt.Sprintf("webhook %s must be configured", known)))
			continue
		}
		if wh.Timeout.Nanoseconds() > (1 * time.Minute).Nanoseconds() {
			allErrs = append(allErrs, field.Invalid(path.Child(known).Child("timeout"), wh.Timeout, fmt.Sprintf("webhook %s invalid, timeout of must be less than or equal to 1m", wh.Name)))
		} else if wh.Timeout.Duration == 0 {
			allErrs = append(allErrs, field.Invalid(path.Child(known).Child("timeout"), wh.Timeout, fmt.Sprintf("webhook %s invalid, timeout of must be specified", wh.Name)))
		}
	}
	return allErrs
}

func validateTargetLoadBalancer(raw *lbcfapi.BackendGroup, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw.Spec.LBName == nil && len(raw.Spec.LoadBalancers) == 0 {
		allErrs = append(allErrs, field.Required(path.Child("loadBalancers"), "loadBalancers must be set"))
	}
	return allErrs
}

func validateBackends(raw *lbcfapi.BackendGroupSpec, path *field.Path) field.ErrorList {
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
	return allErrs
}

func validateServiceBackend(raw *lbcfapi.ServiceBackend, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validatePortSelector(raw.Port, path.Child("port"))...)
	allErrs = append(allErrs, validateLabelSelector(raw.NodeSelector, path.Child("nodeSelector"))...)
	return allErrs
}

func validatePodBackend(raw *lbcfapi.PodBackend, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw.Port == nil && len(raw.Ports) == 0 {
		allErrs = append(allErrs, field.Required(path.Child("ports"), "ports must be specified"))
	}
	for i, port := range raw.Ports {
		allErrs = append(allErrs, validatePortSelector(port, path.Child(fmt.Sprintf("ports[%d]", i)))...)
	}
	if raw.ByLabel != nil {
		if raw.ByName != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("byName"), raw.ByName, "only one of \"byLabel, byName\" is allowed"))
		}
		if len(raw.ByLabel.Selector) == 0 {
			allErrs = append(allErrs, field.Required(path.Child("byLabel").Child("selector"), "selector must be specified"))
		}
		allErrs = append(allErrs, validateLabelSelector(raw.ByLabel.Selector, path.Child("byLabel").Child("selector"))...)
		return allErrs
	}

	if raw.ByName == nil {
		allErrs = append(allErrs, field.Required(path.Child("byLabel/byName"), "one of \"byLabel, byName\" must be specified"))
	}
	return allErrs
}

func validatePortSelector(raw lbcfapi.PortSelector, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if raw.GetPort() <= 0 || raw.GetPort() > 65535 {
		allErrs = append(allErrs, field.Invalid(path.Child("port"), raw.PortNumber, "port must be greater than 0 and less than 65536"))
	}

	if raw.Protocol != string(v1.ProtocolTCP) && raw.Protocol != string(v1.ProtocolUDP) {
		allErrs = append(allErrs, field.Invalid(path.Child("protocol"), raw.Protocol, "protocol must be \"TCP\" or \"UDP\""))
	}
	return allErrs
}

func validateLabelSelector(raw map[string]string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for k, v := range raw {
		_, err := labels.NewRequirement(k, selection.Equals, []string{v})
		if err != nil {
			allErrs = append(allErrs, field.Invalid(path, fmt.Sprintf("%v:%v", k, v), fmt.Sprintf("invalid label: %v", err)))
		}
	}
	return allErrs
}
