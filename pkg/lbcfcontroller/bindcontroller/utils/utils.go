package utils

import (
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	lbcfv1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"
	"tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
)

// MakeBackendLabels generates labels for BackendRecord
func MakeBackendLabels(driverName, bindName, lbName, podName string) map[string]string {
	ret := make(map[string]string)
	ret[lbcfv1.LabelDriverName] = driverName
	ret[lbcfv1.LabelBindName] = bindName
	ret[lbcfv1.LabelLBName] = lbName
	ret[lbcfv1.LabelPodName] = podName
	return ret
}

// ConvertEnsurePolicy converts EnsurePolicyConfig of v1 to v1beta1
func ConvertEnsurePolicy(v1 *lbcfv1.EnsurePolicyConfig) *v1beta1.EnsurePolicyConfig {
	if v1 == nil {
		return nil
	}
	var policy v1beta1.EnsurePolicyType
	switch v1.Policy {
	case lbcfv1.PolicyIfNotSucc:
		policy = v1beta1.PolicyIfNotSucc
	case lbcfv1.PolicyAlways:
		policy = v1beta1.PolicyAlways
	default:
		policy = v1beta1.PolicyIfNotSucc
	}

	var period *v1beta1.Duration
	if v1.ResyncPeriodInSeconds != nil {
		period = &v1beta1.Duration{
			Duration: time.Duration(*v1.ResyncPeriodInSeconds) * time.Second,
		}
	}
	return &v1beta1.EnsurePolicyConfig{
		Policy:    policy,
		MinPeriod: period,
	}
}

// CompareBackendRecords compares expect with have and returns actions should be taken to meet the expect.
//
// The actions are return in 3 BackendRecord slices:
//
// needCreate: BackendsRecords in this slice doesn't exist in K8S and should be created
//
// needUpdate: BackendsReocrds in this slice already exist in K8S and should be update to k8s
//
// needDelete: BackendsRecords in this slice should be deleted from k8s
func CompareBackendRecords(
	expect []*v1beta1.BackendRecord,
	have []*v1beta1.BackendRecord,
	doNotDelete sets.String) (needCreate, needUpdate, needDelete []*v1beta1.BackendRecord) {
	expectedRecords := make(map[string]*v1beta1.BackendRecord)
	for _, e := range expect {
		expectedRecords[e.Name] = e
	}
	haveRecords := make(map[string]*v1beta1.BackendRecord)
	for _, h := range have {
		haveRecords[h.Name] = h
	}
	for k, expect := range expectedRecords {
		have, ok := haveRecords[k]
		if !ok {
			needCreate = append(needCreate, expect)
			continue
		}
		//if !util.EqualStringMap(v.Spec.Parameters, cur.Spec.Parameters) {
		if needUpdateRecord(have, expect) {
			update := have.DeepCopy()
			update.Spec = expect.Spec
			needUpdate = append(needUpdate, update)
		}
	}
	for k, v := range haveRecords {
		if _, ok := expectedRecords[k]; !ok {
			if !doNotDelete.Has(v.Name) {
				needDelete = append(needDelete, v)
			}
		}
	}
	return
}

// IsLoadBalancerCreated returns true if the Created condition is True
func IsLoadBalancerCreated(status lbcfv1.TargetLoadBalancerStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == lbcfv1.LBCreated && cond.Status == lbcfv1.ConditionTrue {
			return true
		}
	}
	return false
}

// IsLoadBalancerReady returns true if the Ready condition is True
func IsLoadBalancerReady(status lbcfv1.TargetLoadBalancerStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == lbcfv1.LBReady && cond.Status == lbcfv1.ConditionTrue {
			return true
		}
	}
	return false
}

// AddOrUpdateLBCondition is an helper function to add specific LoadBalancer condition.
// If a condition with same type exists, the existing one will be overwritten, otherwise, a new condition will be inserted.
func AddOrUpdateLBCondition(
	cur []lbcfv1.TargetLoadBalancerCondition,
	delta lbcfv1.TargetLoadBalancerCondition) []lbcfv1.TargetLoadBalancerCondition {
	cpy := append(make([]lbcfv1.TargetLoadBalancerCondition, 0), cur...)
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

func needUpdateRecord(curObj *v1beta1.BackendRecord, expectObj *v1beta1.BackendRecord) bool {
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
