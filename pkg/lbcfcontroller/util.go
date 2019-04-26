/*
 * Copyright 2019 THL A29 Limited, a Tencent company.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package lbcfcontroller

import (
	"encoding/json"
	"fmt"
	corev1 "k8s.io/client-go/listers/core/v1"
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
		if newStatus.Conditions[i].Type == expectCondition.Type {
			found = true
			newStatus.Conditions[i] = expectCondition
			break
		}
	}
	if !found {
		newStatus.Conditions = append(newStatus.Conditions, expectCondition)
	}
	return *newStatus
}

func addBackendCondition(beStatus *lbcfapi.BackendRecordStatus, expectCondition lbcfapi.BackendRecordCondition) lbcfapi.BackendRecordStatus{
	cpy := beStatus.DeepCopy()
	found := false
	for i := range cpy.Conditions{
		if cpy.Conditions[i].Type != expectCondition.Type{
			found = true
			cpy.Conditions[i] = expectCondition
			break
		}
	}
	if !found{
		cpy.Conditions = append(cpy.Conditions, expectCondition)
	}
	return *cpy
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

func NewPodProvider(lister corev1.PodLister) PodProvider{
	return &PodProviderImpl{
		lister: lister,
	}
}

type PodProvider interface {
	GetPod(name string, namespace string) (*v1.Pod, error)
}

type PodProviderImpl struct {
	lister corev1.PodLister
}

func (p *PodProviderImpl) GetPod(name string, namespace string) (*v1.Pod, error){
	return p.lister.Pods(namespace).Get(name)
}

func containerPortToK8sContainerPort(port lbcfapi.ContainerPort) v1.ContainerPort{
	return v1.ContainerPort{
		Name: port.Name,
		HostPort: port.HostPort,
		ContainerPort: port.ContainerPort,
		Protocol: v1.Protocol(port.Protocol),
		HostIP: port.HostIP,
	}
}

func recordIndex(obj interface{}) (string, error) {
	r := obj.(*lbcfapi.BackendRecord)
	index, err := json.Marshal(r.Spec.LBInfo)
	return string(index), err
}

func hasFinalizer(all []string, expect string) bool{
	for i := range all{
		if all[i] == expect{
			return true
		}
	}
	return false
}


func removeFinalizer(all []string, toDelete string) []string{
	var ret []string
	for i := range all{
		if all[i] != toDelete {
			ret = append(ret, all[i])
		}
	}
	return ret
}

func namespacedNameKeyFunc(namespace, name string) string{
	if len(namespace) > 0{
		return namespace + "/" + name
	}
	return name
}