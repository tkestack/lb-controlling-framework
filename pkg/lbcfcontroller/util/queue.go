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

package util

import (
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

// ConditionalRateLimitingInterface is an workqueue.RateLimitingInterface that can Add item with a filter
type ConditionalRateLimitingInterface interface {
	workqueue.DelayingInterface
	AddAfterMinimumDelay(item interface{}, duration time.Duration)
	Forget(item interface{})
	AddAfterFiltered(item interface{}, duration time.Duration)
	LenWaitingForFilter() int
}

// NewConditionalDelayingQueue returns a new instance of ConditionalRateLimitingInterface. If minDelay is less than step, the real minimum delay is step.
func NewConditionalDelayingQueue(filter QueueFilter, minDelay time.Duration, step time.Duration, maxDelay time.Duration) ConditionalRateLimitingInterface {
	rateLimiter := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(step, maxDelay),
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		&fixedRateDelayLimiter{duration: minDelay},
	)
	q := &conditionalRateLimitingQueue{
		DelayingInterface:      workqueue.NewDelayingQueue(),
		rateLimiter:            rateLimiter,
		waitingWithFilterQueue: workqueue.NewDelayingQueue(),
		filter:                 filter,
		minDelay:               minDelay,
	}

	go q.run()

	return q
}

type conditionalRateLimitingQueue struct {
	workqueue.DelayingInterface
	rateLimiter            workqueue.RateLimiter
	waitingWithFilterQueue workqueue.DelayingInterface
	filter                 QueueFilter
	minDelay               time.Duration
}

// AddAfterMinimumDelay adds item after at least the indicated minDelay has passed
func (q *conditionalRateLimitingQueue) AddAfterMinimumDelay(item interface{}, minDelay time.Duration) {
	delay := q.rateLimiter.When(item)
	if minDelay.Nanoseconds() > delay.Nanoseconds() {
		delay = minDelay
	}
	q.DelayingInterface.AddAfter(item, delay)
}

// Forget indicates that an item is finished being retried
func (q *conditionalRateLimitingQueue) Forget(item interface{}) {
	q.rateLimiter.Forget(item)
}

// AddAfterFiltered adds item after the indicated duration has passed, a filter will run on the item before Get
func (q *conditionalRateLimitingQueue) AddAfterFiltered(item interface{}, duration time.Duration) {
	if duration.Nanoseconds() < q.minDelay.Nanoseconds() {
		duration = q.minDelay
	}
	q.waitingWithFilterQueue.AddAfter(item, duration)
}

// LenWaitingForFilter returns the number of items that not yet filtered
func (q *conditionalRateLimitingQueue) LenWaitingForFilter() int {
	return q.waitingWithFilterQueue.Len()
}

func (q *conditionalRateLimitingQueue) run() {
	for q.filterQueue() {
	}
}

func (q *conditionalRateLimitingQueue) filterQueue() bool {
	item, quit := q.waitingWithFilterQueue.Get()
	if quit {
		klog.Warningf("conditionalRateLimitingQueue quit")
		return false
	}
	q.waitingWithFilterQueue.Done(item)

	if q.filter == nil {
		q.Add(item)
		return true
	}

	match, err := q.filter(item)
	if err != nil {
		klog.Errorf("conditionalRateLimitingQueue filter %v failed: %v", item, err)
		q.AddAfterFiltered(item, q.minDelay)
		return true
	}
	if match {
		q.Add(item)
	}
	return true
}

type fixedRateDelayLimiter struct {
	duration time.Duration
}

func (l *fixedRateDelayLimiter) When(item interface{}) time.Duration {
	return l.duration
}

func (l *fixedRateDelayLimiter) Forget(item interface{}) {
}

func (l *fixedRateDelayLimiter) NumRequeues(item interface{}) int {
	return 0
}

// QueueFilter is a function that filter queue elements
type QueueFilter func(item interface{}) (bool, error)

// QueueFilterForLB returns aPeriodicFilter for LoadBalancer
func QueueFilterForLB(lbLister v1beta1.LoadBalancerLister) QueueFilter {
	return func(item interface{}) (bool, error) {
		key := item.(string)
		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			return false, err
		}
		lb, err := lbLister.LoadBalancers(namespace).Get(name)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if NeedPeriodicEnsure(lb.Spec.EnsurePolicy, lb.DeletionTimestamp != nil) {
			return true, nil
		}
		return false, nil
	}
}

// QueueFilterForBackend returns a PeriodicFilter for BackendRecord
func QueueFilterForBackend(backendLister v1beta1.BackendRecordLister) QueueFilter {
	return func(item interface{}) (bool, error) {
		key := item.(string)
		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			return false, err
		}
		backend, err := backendLister.BackendRecords(namespace).Get(name)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if NeedPeriodicEnsure(backend.Spec.EnsurePolicy, backend.DeletionTimestamp != nil) {
			return true, nil
		}
		return false, nil
	}
}
