package v1

import (
	v1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"
)

// DeregIfNotRunning returns true if backend should be deregistered when not running, instead of not ready
func DeregIfNotRunning(bind *v1.Bind) bool {
	if bind.Spec.DeregisterPolicy != nil && *bind.Spec.DeregisterPolicy == v1.DeregisterIfNotRunning {
		return true
	}
	return false
}

// DeregByWebhook returns true if the webhook should determine if the backend should be deregistered
func DeregByWebhook(bind *v1.Bind) bool {
	if bind.Spec.DeregisterPolicy != nil &&
		*bind.Spec.DeregisterPolicy == v1.DeregisterWebhook &&
		bind.Spec.DeregisterWebhook != nil {
		return true
	}
	return false
}
