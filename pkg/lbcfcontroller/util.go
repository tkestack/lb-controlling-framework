package lbcfcontroller

import (
	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/api/v1/pod"
	"strings"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

func DefaultControllerRateLimiter() workqueue.RateLimiter {
	return workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(10*time.Second, 10*time.Minute),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}

func podAvailable(obj *v1.Pod) bool {
	return obj.Status.PodIP != "" && pod.IsPodReady(obj)
}

func lbCreated(lb *lbcfapi.LoadBalancer) bool {
	condition := getLBCondition(&lb.Status, lbcfapi.LBCreated)
	return condition == lbcfapi.ConditionTrue
}

func getLBCondition(status *lbcfapi.LoadBalancerStatus, conditionType lbcfapi.LoadBalancerConditionType) lbcfapi.ConditionStatus {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return status.Conditions[i].Status
		}
	}
	return lbcfapi.ConditionFalse
}

type BackendType string

const(
	TypeService BackendType = "Service"
	TypePod BackendType = "Pod"
	TypeStatic BackendType = "Static"
	TypeUnknown BackendType = "Unknown"
)


func getBackendType(bg *lbcfapi.BackendGroup) BackendType{
	if bg.Spec.Pods != nil{
		return TypePod
	}else if bg.Spec.Service != nil{
		return TypeService
	}else if bg.Spec.Service != nil{
		return TypeStatic
	}
	return TypeUnknown
}

func getDriverNamespace(lb *lbcfapi.LoadBalancer) string{
	if strings.HasPrefix(lb.Spec.LBDriver, SystemDriverPrefix){
		return "kube-system"
	}
	return lb.Namespace
}