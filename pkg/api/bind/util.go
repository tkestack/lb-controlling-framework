package bind

import (
	"reflect"
	"time"

	v1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"
	"tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
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

// ConvertEnsurePolicy converts EnsurePolicyConfig of v1 to v1beta1
func ConvertEnsurePolicy(ensurePolicy *v1.EnsurePolicyConfig) *v1beta1.EnsurePolicyConfig {
	if ensurePolicy == nil {
		return nil
	}
	var policy v1beta1.EnsurePolicyType
	switch ensurePolicy.Policy {
	case v1.PolicyIfNotSucc:
		policy = v1beta1.PolicyIfNotSucc
	case v1.PolicyAlways:
		policy = v1beta1.PolicyAlways
	default:
		policy = v1beta1.PolicyIfNotSucc
	}

	var period *v1beta1.Duration
	if ensurePolicy.ResyncPeriodInSeconds != nil {
		period = &v1beta1.Duration{
			Duration: time.Duration(*ensurePolicy.ResyncPeriodInSeconds) * time.Second,
		}
	}
	return &v1beta1.EnsurePolicyConfig{
		Policy:    policy,
		MinPeriod: period,
	}
}

// IsLoadBalancerCreated returns true if the Created condition is True
func IsLoadBalancerCreated(status v1.TargetLoadBalancerStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == v1.LBCreated && cond.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

// IsLoadBalancerReady returns true if the Ready condition is True
func IsLoadBalancerReady(status v1.TargetLoadBalancerStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == v1.LBReady && cond.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

// AddOrUpdateLBCondition is an helper function to add specific LoadBalancer condition.
// If a condition with same type exists, the existing one will be overwritten, otherwise, a new condition will be inserted.
func AddOrUpdateLBCondition(
	cur []v1.TargetLoadBalancerCondition,
	delta v1.TargetLoadBalancerCondition) []v1.TargetLoadBalancerCondition {
	cpy := append(make([]v1.TargetLoadBalancerCondition, 0), cur...)
	found := false
	for i := range cpy {
		if cpy[i].Type == delta.Type {
			found = true
			cpy[i] = delta
			break
		}
	}
	if !found {
		cpy = append(cpy, delta)
	}
	return cpy
}

func NeedUpdateRecord(curObj *v1beta1.BackendRecord, expectObj *v1beta1.BackendRecord) bool {
	if !reflect.DeepEqual(curObj.Spec.LBAttributes, expectObj.Spec.LBAttributes) {
		return true
	}
	if !reflect.DeepEqual(curObj.Spec.Parameters, expectObj.Spec.Parameters) {
		return true
	}
	if !reflect.DeepEqual(curObj.Spec.EnsurePolicy, expectObj.Spec.EnsurePolicy) {
		return true
	}
	return false
}
