package lbcfcontroller

import (
	"fmt"
	"k8s.io/klog"
	"strings"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"golang.org/x/time/rate"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/api/v1/pod"
)

func DefaultControllerRateLimiter() workqueue.RateLimiter {
	return workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(DefaultRetryInterval, 10*time.Minute),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}


type IntervalRateLimitingInterface interface {
	workqueue.RateLimitingInterface

	AddIntervalRateLimited(item interface{}, minInterval time.Duration)
}

func NewIntervalRateLimitingQueue(rateLimiter workqueue.RateLimiter, name string) IntervalRateLimitingInterface{
	return &IntervalRateLimitingQueue{
		DelayingInterface: workqueue.NewNamedDelayingQueue(name),
		rateLimiter: rateLimiter,
	}
}

type IntervalRateLimitingQueue struct {
	workqueue.DelayingInterface

	rateLimiter workqueue.RateLimiter
}

func (q *IntervalRateLimitingQueue) AddIntervalRateLimited(item interface{}, minInterval time.Duration){
	delay := q.rateLimiter.When(item)
	if minInterval.Nanoseconds() > delay.Nanoseconds(){
		delay = minInterval
	}
	q.DelayingInterface.AddAfter(item, delay)
}

// AddRateLimited AddAfter's the item based on the time when the rate limiter says its ok
func (q *IntervalRateLimitingQueue) AddRateLimited(item interface{}) {
	q.DelayingInterface.AddAfter(item, q.rateLimiter.When(item))
}

func (q *IntervalRateLimitingQueue) NumRequeues(item interface{}) int {
	return q.rateLimiter.NumRequeues(item)
}

func (q *IntervalRateLimitingQueue) Forget(item interface{}) {
	q.rateLimiter.Forget(item)
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

func addLBCondition(lbStatus *lbcfapi.LoadBalancerStatus, expectCondition lbcfapi.LoadBalancerCondition) lbcfapi.LoadBalancerStatus {
	newStatus := lbStatus.DeepCopy()
	found := false
	for i := range newStatus.Conditions {
		if newStatus.Conditions[i].Type != expectCondition.Type {
			continue
		}
		found = true
		newStatus.Conditions[i] = expectCondition
	}
	if !found {
		newStatus.Conditions = append(newStatus.Conditions, expectCondition)
	}
	return *newStatus
}


type BackendType string

const (
	TypeService BackendType = "Service"
	TypePod     BackendType = "Pod"
	TypeStatic  BackendType = "Static"
	TypeUnknown BackendType = "Unknown"
)

func getBackendType(bg *lbcfapi.BackendGroup) BackendType {
	if bg.Spec.Pods != nil {
		return TypePod
	} else if bg.Spec.Service != nil {
		return TypeService
	} else if bg.Spec.Service != nil {
		return TypeStatic
	}
	return TypeUnknown
}

func getDriverNamespace(driverName string, defaultNamespace string) string {
	if strings.HasPrefix(driverName, SystemDriverPrefix) {
		return "kube-system"
	}
	return defaultNamespace
}

func isDriverDraining(labels map[string]string) bool {
	if v, ok := labels[driverDrainingLabel]; !ok || strings.ToUpper(v) != "TRUE" {
		return true
	}
	return false
}

func calculateRetryInterval(defaultInterval time.Duration, userValueInSeconds int32) time.Duration{
	if userValueInSeconds == 0{
		return defaultInterval
	}
	dur, err := time.ParseDuration(fmt.Sprintf("%ds", userValueInSeconds))
	if err != nil{
		klog.Warningf("parse retryIntervalInSeconds failed: %v", err)
		return defaultInterval
	}
	return dur
}