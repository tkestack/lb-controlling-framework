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
	"fmt"
	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
	"testing"
	"time"
)

func TestConditionalRateLimitingQueueAddAfterMinimumDelay(t *testing.T) {
	q := NewConditionalDelayingQueue(nil, time.Millisecond, time.Millisecond, time.Millisecond)
	startTime := time.Now()
	q.AddAfterMinimumDelay(struct{}{}, time.Second)
	_, quit := q.Get()
	if quit {
		t.Fatalf("should not quit")
	}
	elapsed := time.Now().Sub(startTime)
	if elapsed.Nanoseconds() < time.Second.Nanoseconds() {
		t.Fatalf("too short, %s", elapsed.String())
	}
}

func TestConditionalRateLimitingQueueAddAfterMinimumDelayWithEmptyValue(t *testing.T) {
	q := NewConditionalDelayingQueue(nil, time.Second, time.Second, time.Second)
	startTime := time.Now()
	q.AddAfterMinimumDelay(struct{}{}, 0)
	_, quit := q.Get()
	if quit {
		t.Fatalf("should not quit")
	}
	elapsed := time.Now().Sub(startTime)
	if elapsed.Nanoseconds() < time.Second.Nanoseconds() {
		t.Fatalf("too short, %s", elapsed.String())
	}
}

func TestConditionalRateLimitingQueueAddAfterFiltered(t *testing.T) {
	type testCase struct {
		name    string
		filter  QueueFilter
		item    interface{}
		output  int
		waiting int
	}
	cases := []testCase{
		{
			name: "match",
			filter: func(item interface{}) (b bool, e error) {
				return true, nil
			},
			item:   struct{}{},
			output: 1,
		},
		{
			name:   "match-with-nil-filter",
			filter: nil,
			item:   struct{}{},
			output: 1,
		},
		{
			name: "no-match",
			filter: func(item interface{}) (b bool, e error) {
				return false, nil
			},
			item: struct{}{},
		},
		{
			name: "no-match-with-err-filter",
			filter: func(item interface{}) (b bool, e error) {
				return false, fmt.Errorf("fake error")
			},
			item:    "test-item",
			waiting: 1,
		},
	}
	for _, c := range cases {
		//q := NewConditionalDelayingQueue(c.filter, time.Millisecond, time.Millisecond, time.Millisecond)
		rateLimiter := workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(time.Millisecond, time.Millisecond),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
			&fixedRateDelayLimiter{duration: time.Millisecond},
		)
		q := &conditionalRateLimitingQueue{
			DelayingInterface:      workqueue.NewDelayingQueue(),
			rateLimiter:            rateLimiter,
			waitingWithFilterQueue: workqueue.NewDelayingQueue(),
			filter:                 c.filter,
			minDelay:               time.Millisecond,
		}
		q.AddAfterFiltered(c.item, 0)
		q.filterQueue()
		if q.Len() != c.output {
			t.Errorf("case %s, expect %v, get %v", c.name, c.output, q.Len())
		}
		if c.waiting > 0 {
			time.Sleep(50 * time.Millisecond)
			if q.LenWaitingForFilter() != c.waiting {
				t.Errorf("case %s, expect %v, get %v", c.name, c.waiting, q.LenWaitingForFilter())
			}
		}
	}
}

func TestQueueFilterForLB(t *testing.T) {
	ts := v1.Now()
	lister := &fakeLBListerWithStore{
		store: map[string]*lbcfapi.LoadBalancer{
			"normal": {
				ObjectMeta: v1.ObjectMeta{
					Name: "normal",
				},
				Spec: lbcfapi.LoadBalancerSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy:    lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{Duration: time.Millisecond},
					},
				},
			},
			"deleting": {
				ObjectMeta: v1.ObjectMeta{
					Name:              "deleting",
					DeletionTimestamp: &ts,
				},
			},
		},
	}
	filter := QueueFilterForLB(lister)
	type testCase struct {
		name        string
		key         string
		expectMatch bool
		expectError bool
	}
	cases := []testCase{
		{
			name:        "match",
			key:         NamespacedNameKeyFunc("", "normal"),
			expectMatch: true,
		},
		{
			name: "no-match-deleting",
			key:  NamespacedNameKeyFunc("", "deleting"),
		},
		{
			name: "no-match-not-found",
			key:  NamespacedNameKeyFunc("", "not-exist"),
		},
	}
	for _, c := range cases {
		match, err := filter(c.key)
		if match != c.expectMatch {
			t.Errorf("case %s, expect %v, get %v", c.name, c.expectMatch, match)
		}
		hasErr := err != nil
		if hasErr != c.expectError {
			t.Errorf("case %s, expect %v, get %v", c.name, c.expectError, hasErr)
		}
	}
}

func TestQueueFilterForBackend(t *testing.T) {
	ts := v1.Now()
	lister := &fakeBackendListerWithStore{
		store: map[string]*lbcfapi.BackendRecord{
			"normal": {
				ObjectMeta: v1.ObjectMeta{
					Name: "normal",
				},
				Spec: lbcfapi.BackendRecordSpec{
					EnsurePolicy: &lbcfapi.EnsurePolicyConfig{
						Policy:    lbcfapi.PolicyAlways,
						MinPeriod: &lbcfapi.Duration{Duration: time.Millisecond},
					},
				},
			},
			"deleting": {
				ObjectMeta: v1.ObjectMeta{
					Name:              "deleting",
					DeletionTimestamp: &ts,
				},
			},
		},
	}
	filter := QueueFilterForBackend(lister)
	type testCase struct {
		name        string
		key         string
		expectMatch bool
		expectError bool
	}
	cases := []testCase{
		{
			name:        "match",
			key:         NamespacedNameKeyFunc("", "normal"),
			expectMatch: true,
		},
		{
			name: "no-match-deleting",
			key:  NamespacedNameKeyFunc("", "deleting"),
		},
		{
			name: "no-match-not-found",
			key:  NamespacedNameKeyFunc("", "not-exist"),
		},
	}
	for _, c := range cases {
		match, err := filter(c.key)
		if match != c.expectMatch {
			t.Errorf("case %s, expect %v, get %v", c.name, c.expectMatch, match)
		}
		hasErr := err != nil
		if hasErr != c.expectError {
			t.Errorf("case %s, expect %v, get %v", c.name, c.expectError, hasErr)
		}
	}
}

type fakeLBListerWithStore struct {
	store map[string]*lbcfapi.LoadBalancer
}

func (l *fakeLBListerWithStore) Get(name string) (*lbcfapi.LoadBalancer, error) {
	if l.store == nil {
		l.store = make(map[string]*lbcfapi.LoadBalancer)
	}
	lb, ok := l.store[name]
	if !ok {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "LoadBalancer",
		}, name)
	}
	return lb, nil
}

func (l *fakeLBListerWithStore) List(selector labels.Selector) (ret []*lbcfapi.LoadBalancer, err error) {
	for _, lb := range l.store {
		ret = append(ret, lb)
	}
	return
}

func (l *fakeLBListerWithStore) LoadBalancers(namespace string) v1beta1.LoadBalancerNamespaceLister {
	return l
}

type fakeBackendListerWithStore struct {
	// map: name -> BackendRecord
	store map[string]*lbcfapi.BackendRecord
}

func (l *fakeBackendListerWithStore) Get(name string) (*lbcfapi.BackendRecord, error) {
	backend, ok := l.store[name]
	if !ok {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "lbcf.tke.cloud.tencent.com/v1beta1",
			Resource: "BackendRecord",
		}, name)
	}
	return backend, nil
}

func (l *fakeBackendListerWithStore) List(selector labels.Selector) (ret []*lbcfapi.BackendRecord, err error) {
	for _, backend := range l.store {
		ret = append(ret, backend)
	}
	return
}

func (l *fakeBackendListerWithStore) BackendRecords(namespace string) v1beta1.BackendRecordNamespaceLister {
	return l
}
