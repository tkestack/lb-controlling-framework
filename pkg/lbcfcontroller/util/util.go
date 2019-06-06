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
	"bytes"
	"crypto/md5"
	"fmt"
	"reflect"
	"strings"
	"text/template"
	"time"

	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"golang.org/x/time/rate"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/api/v1/pod"
)

const (
	// DefaultRetryInterval is the default minimum delay for retries
	DefaultRetryInterval = 10 * time.Second

	// DefaultEnsurePeriod is the default minimum interval for ensureLoadBalancer and ensureBackendRecord
	DefaultEnsurePeriod = 1 * time.Minute
)

// DefaultControllerRateLimiter creates a RateLimiter with DefaultRetryInterval as it's base delay
func DefaultControllerRateLimiter() workqueue.RateLimiter {
	return workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(DefaultRetryInterval, 10*time.Minute),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}

// IntervalRateLimitingInterface limits rate with rate limiter and user specified minimum interval
type IntervalRateLimitingInterface interface {
	workqueue.RateLimitingInterface

	// AddIntervalRateLimited adds object in at least minInterval
	AddIntervalRateLimited(item interface{}, minInterval time.Duration)
}

// NewIntervalRateLimitingQueue creates a new instance of IntervalRateLimitingInterface,
// defaultDelay is used as default minimum interval for retries
func NewIntervalRateLimitingQueue(rateLimiter workqueue.RateLimiter, name string, defaultDelay time.Duration) IntervalRateLimitingInterface {
	return &IntervalRateLimitingQueue{
		defaultRetryDelay: defaultDelay,
		DelayingInterface: workqueue.NewNamedDelayingQueue(name),
		rateLimiter:       rateLimiter,
	}
}

// IntervalRateLimitingQueue is an implementation of IntervalRateLimitingInterface
type IntervalRateLimitingQueue struct {
	defaultRetryDelay time.Duration

	workqueue.DelayingInterface

	rateLimiter workqueue.RateLimiter
}

// AddIntervalRateLimited adds item in at least minInterval
func (q *IntervalRateLimitingQueue) AddIntervalRateLimited(item interface{}, minInterval time.Duration) {
	if minInterval.Nanoseconds() == 0 {
		minInterval = q.defaultRetryDelay
	}
	delay := q.rateLimiter.When(item)
	if minInterval.Nanoseconds() > delay.Nanoseconds() {
		delay = minInterval
	}
	q.DelayingInterface.AddAfter(item, delay)
}

// AddRateLimited AddAfters the item based on the time when the rate limiter says its ok
func (q *IntervalRateLimitingQueue) AddRateLimited(item interface{}) {
	q.DelayingInterface.AddAfter(item, q.rateLimiter.When(item))
}

// NumRequeues returns back how many failures the item has had
func (q *IntervalRateLimitingQueue) NumRequeues(item interface{}) int {
	return q.rateLimiter.NumRequeues(item)
}

// Forget indicates that an item is finished being retried.  Doesn't matter whether its for perm failing
// or for success, we'll stop tracking it
func (q *IntervalRateLimitingQueue) Forget(item interface{}) {
	q.rateLimiter.Forget(item)
}

// PodAvailable indicates the given pod is ready to bind to load balancers
func PodAvailable(obj *v1.Pod) bool {
	return obj.Status.PodIP != "" && obj.DeletionTimestamp == nil && pod.IsPodReady(obj)
}

// LBCreated indicates the given LoadBalancer is successfully created by webhook createLoadBalancer
func LBCreated(lb *lbcfapi.LoadBalancer) bool {
	condition := GetLBCondition(&lb.Status, lbcfapi.LBCreated)
	if condition == nil {
		return false
	}
	return condition.Status == lbcfapi.ConditionTrue
}

// LBEnsured indicates the given LoadBalancer is successfully ensured by webhook ensureLoadBalancer
func LBEnsured(lb *lbcfapi.LoadBalancer) bool {
	condition := GetLBCondition(&lb.Status, lbcfapi.LBAttributesSynced)
	if condition == nil {
		return false
	}
	return condition.Status == lbcfapi.ConditionTrue
}

// LBNeedEnsure indicates webhook ensureLoadBalancer should be called on the given LoadBalancer
func LBNeedEnsure(lb *lbcfapi.LoadBalancer) bool {
	if !LBEnsured(lb) {
		return true
	}
	return lb.Spec.EnsurePolicy != nil && lb.Spec.EnsurePolicy.Policy == lbcfapi.PolicyAlways
}

// GetLBCondition is an helper function to get specific LoadBalancer condition
func GetLBCondition(status *lbcfapi.LoadBalancerStatus, conditionType lbcfapi.LoadBalancerConditionType) *lbcfapi.LoadBalancerCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}
	return nil
}

// AddLBCondition is an helper function to add specific LoadBalancer condition into LoadBalancer.status.
// If a condition with same type exists, the existing one will be overwritten, otherwise, a new condition will be inserted.
func AddLBCondition(lbStatus *lbcfapi.LoadBalancerStatus, expectCondition lbcfapi.LoadBalancerCondition) {
	found := false
	for i := range lbStatus.Conditions {
		if lbStatus.Conditions[i].Type == expectCondition.Type {
			found = true
			lbStatus.Conditions[i] = expectCondition
			break
		}
	}
	if !found {
		lbStatus.Conditions = append(lbStatus.Conditions, expectCondition)
	}
}

// GetBackendRecordCondition is an helper function to get specific BackendRecord condition
func GetBackendRecordCondition(status *lbcfapi.BackendRecordStatus, conditionType lbcfapi.BackendRecordConditionType) *lbcfapi.BackendRecordCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}
	return nil
}

// AddBackendCondition is an helper function to add specific BackendRecord condition into BackendRecord.status.
// If a condition with same type exists, the existing one will be overwritten, otherwise, a new condition will be inserted.
func AddBackendCondition(beStatus *lbcfapi.BackendRecordStatus, expectCondition lbcfapi.BackendRecordCondition) {
	found := false
	for i := range beStatus.Conditions {
		if beStatus.Conditions[i].Type == expectCondition.Type {
			found = true
			beStatus.Conditions[i] = expectCondition
			break
		}
	}
	if !found {
		beStatus.Conditions = append(beStatus.Conditions, expectCondition)
	}
}

// BackendType indicates the elements that form a BackendGroup
type BackendType string

const (
	// TypeService indicates the BackendGroup consists of services
	TypeService BackendType = "Service"

	// TypePod indicates the BackendGroup consists of pods
	TypePod BackendType = "Pod"

	// TypeStatic indicates the BackendGroup consists of static addresses
	TypeStatic BackendType = "Static"

	// TypeUnknown indicates the BackendGroup consists of unknown backends
	TypeUnknown BackendType = "Unknown"
)

// GetBackendType returns the BackendType of bg
func GetBackendType(bg *lbcfapi.BackendGroup) BackendType {
	if bg.Spec.Pods != nil {
		return TypePod
	} else if bg.Spec.Service != nil {
		return TypeService
	}
	return TypeStatic
}

// GetDriverNamespace returns the namespace where the driver is created.
// It returns "kube-system" if driverName starts with "lbcf-", otherwise the defaultNamespace is returned
func GetDriverNamespace(driverName string, defaultNamespace string) string {
	if strings.HasPrefix(driverName, lbcfapi.SystemDriverPrefix) {
		return metav1.NamespaceSystem
	}
	return defaultNamespace
}

// IsDriverDraining indicates whether driver is draining
func IsDriverDraining(driver *lbcfapi.LoadBalancerDriver) bool {
	if v, ok := driver.Labels[lbcfapi.DriverDrainingLabel]; !ok || strings.ToUpper(v) != "TRUE" {
		return false
	}
	return true
}

// CalculateRetryInterval converts userValueInSeconds to time.Duration,
// it returns DefaultRetryInterval if userValueInSeconds is not specified
func CalculateRetryInterval(userValueInSeconds int32) time.Duration {
	if userValueInSeconds <= 0 {
		return DefaultRetryInterval
	}
	dur, err := time.ParseDuration(fmt.Sprintf("%ds", userValueInSeconds))
	if err != nil {
		klog.Warningf("parse retryIntervalInSeconds failed: %v", err)
		return DefaultRetryInterval
	}
	return dur
}

// HasFinalizer is an helper function to look for expect in all
func HasFinalizer(all []string, expect string) bool {
	for i := range all {
		if all[i] == expect {
			return true
		}
	}
	return false
}

// RemoveFinalizer removes toDelete from all and returns a new slice
func RemoveFinalizer(all []string, toDelete string) []string {
	var ret []string
	for i := range all {
		if all[i] != toDelete {
			ret = append(ret, all[i])
		}
	}
	return ret
}

// NamespacedNameKeyFunc generates a name that can be handled by cache.DeletionHandlingMetaNamespaceKeyFunc
func NamespacedNameKeyFunc(namespace, name string) string {
	if len(namespace) > 0 {
		return namespace + "/" + name
	}
	return name
}

// GetDuration converts cfg to time.Duration, defaultValue is returned if cfg is nil
func GetDuration(cfg *lbcfapi.Duration, defaultValue time.Duration) time.Duration {
	if cfg == nil {
		return defaultValue
	}
	return cfg.Duration
}

// LoadBalancerNonStatusUpdated returns true if non-status fields are modified
func LoadBalancerNonStatusUpdated(old *lbcfapi.LoadBalancer, cur *lbcfapi.LoadBalancer) bool {
	if !reflect.DeepEqual(old.Spec.Attributes, cur.Spec.Attributes) {
		return true
	}
	return false
}

// BackendGroupNonStatusUpdated returns true if non-status fields are modified
func BackendGroupNonStatusUpdated(old *lbcfapi.BackendGroup, cur *lbcfapi.BackendGroup) bool {
	if bgServiceUpdated(old, cur) {
		return true
	}
	if bgPodsUpdated(old, cur) {
		return true
	}
	if bgStaticUpdated(old, cur) {
		return true
	}
	if !reflect.DeepEqual(old.Spec.Parameters, cur.Spec.Parameters) {
		return true
	}
	return false
}

// TODO: implement this
func bgServiceUpdated(old *lbcfapi.BackendGroup, cur *lbcfapi.BackendGroup) bool {
	return false
}

func bgPodsUpdated(old *lbcfapi.BackendGroup, cur *lbcfapi.BackendGroup) bool {
	return !reflect.DeepEqual(old.Spec.Pods, cur.Spec.Pods)
}

func bgStaticUpdated(old *lbcfapi.BackendGroup, cur *lbcfapi.BackendGroup) bool {
	if !reflect.DeepEqual(old.Spec.Static, cur.Spec.Static) {
		return true
	}
	return false
}

// ErrorList is an helper that collects errors
type ErrorList []error

// Error returns formats all errors into one error
func (e ErrorList) Error() string {
	var msg []string
	for i, err := range e {
		msg = append(msg, fmt.Sprintf("%d: %v", i+1, err))
	}
	return strings.Join(msg, "\n")
}

// MakePodBackendName generates a name for BackendRecord
func MakePodBackendName(lbName, groupName string, podUID types.UID, port lbcfapi.PortSelector) string {
	raw := fmt.Sprintf("%s_%s_%s_%d_%+v", lbName, groupName, podUID, port.PortNumber, GetPortSelectorProto(port))
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", h)
}

// MakeServiceBackendName generates a name for BackendRecord of service type
func MakeServiceBackendName(lbName, groupName, svcName string, nodePort int32, nodePortProtocol string, nodeName string) string {
	raw := fmt.Sprintf("%s_%s_%s_%d_%s_%s", lbName, groupName, svcName, nodePort, nodePortProtocol, nodeName)
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", h)
}

// MakeStaticBackendName generates a name for BackendRecord of service type
func MakeStaticBackendName(lbName, groupName, staticAddr string) string {
	raw := fmt.Sprintf("%s_%s_%s", lbName, groupName, staticAddr)
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", h)
}

// MakeBackendLabels generates labels for BackendRecord
func MakeBackendLabels(driverName, lbName, groupName, svcName, podName, staticAddr string) map[string]string {
	ret := make(map[string]string)
	ret[lbcfapi.LabelDriverName] = driverName
	ret[lbcfapi.LabelLBName] = lbName
	ret[lbcfapi.LabelGroupName] = groupName
	if podName != "" {
		ret[lbcfapi.LabelPodName] = podName
	}
	if svcName != "" {
		ret[lbcfapi.LabelServiceName] = svcName
	}
	if staticAddr != "" {
		ret[lbcfapi.LabelStaticAddr] = staticAddr
	}
	return ret
}

// ConstructPodBackendRecord constructs a new BackendRecord
func ConstructPodBackendRecord(lb *lbcfapi.LoadBalancer, group *lbcfapi.BackendGroup, pod *v1.Pod) *lbcfapi.BackendRecord {
	valueTrue := true
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakePodBackendName(lb.Name, group.Name, pod.UID, group.Spec.Pods.Port),
			Namespace: group.Namespace,
			Labels:    MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, "", pod.Name, ""),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         lbcfapi.ApiVersion,
					BlockOwnerDeletion: &valueTrue,
					Controller:         &valueTrue,
					Kind:               "BackendGroup",
					Name:               group.Name,
					UID:                group.UID,
				},
			},
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:       lb.Name,
			LBDriver:     lb.Spec.LBDriver,
			LBInfo:       lb.Status.LBInfo,
			LBAttributes: lb.Spec.Attributes,
			PodBackendInfo: &lbcfapi.PodBackendRecord{
				Name: pod.Name,
				Port: group.Spec.Pods.Port,
			},
			Parameters:   group.Spec.Parameters,
			EnsurePolicy: group.Spec.EnsurePolicy,
		},
	}
}

// ConstructServiceBackendRecord constructs a new BackendRecord of type service
func ConstructServiceBackendRecord(lb *lbcfapi.LoadBalancer, group *lbcfapi.BackendGroup, svc *v1.Service, node *v1.Node) *lbcfapi.BackendRecord {
	var selectedSvcPort *v1.ServicePort
	wantedPort := group.Spec.Service.Port
	for i, svcPort := range svc.Spec.Ports {
		if svcPort.Port == wantedPort.PortNumber && strings.ToUpper(string(svcPort.Protocol)) == GetPortSelectorProto(wantedPort) {
			selectedSvcPort = &svc.Spec.Ports[i]
			break
		}
	}
	if selectedSvcPort == nil || selectedSvcPort.NodePort == 0 {
		return nil
	}

	valueTrue := true
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeServiceBackendName(lb.Name, group.Name, svc.Name, selectedSvcPort.Port, string(selectedSvcPort.Protocol), node.Name),
			Namespace: group.Namespace,
			Labels:    MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, svc.Name, "", ""),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         lbcfapi.ApiVersion,
					BlockOwnerDeletion: &valueTrue,
					Controller:         &valueTrue,
					Kind:               "BackendGroup",
					Name:               group.Name,
					UID:                group.UID,
				},
			},
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:       lb.Name,
			LBDriver:     lb.Spec.LBDriver,
			LBInfo:       lb.Status.LBInfo,
			LBAttributes: lb.Spec.Attributes,
			ServiceBackendInfo: &lbcfapi.ServiceBackendRecord{
				Name:     svc.Name,
				Port:     group.Spec.Service.Port,
				NodePort: selectedSvcPort.NodePort,
				NodeName: node.Name,
			},
			Parameters:   group.Spec.Parameters,
			EnsurePolicy: group.Spec.EnsurePolicy,
		},
	}
}

// ConstructStaticBackend constructs BackendRecords of type service
func ConstructStaticBackend(lb *lbcfapi.LoadBalancer, group *lbcfapi.BackendGroup, staticAddr string) *lbcfapi.BackendRecord {
	valueTrue := true
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeStaticBackendName(lb.Name, group.Name, staticAddr),
			Namespace: group.Namespace,
			Labels:    MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, "", "", staticAddr),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         lbcfapi.ApiVersion,
					BlockOwnerDeletion: &valueTrue,
					Controller:         &valueTrue,
					Kind:               "BackendGroup",
					Name:               group.Name,
					UID:                group.UID,
				},
			},
		},
		Spec: lbcfapi.BackendRecordSpec{
			LBName:       lb.Name,
			LBDriver:     lb.Spec.LBDriver,
			LBInfo:       lb.Status.LBInfo,
			LBAttributes: lb.Spec.Attributes,
			Parameters:   group.Spec.Parameters,
			EnsurePolicy: group.Spec.EnsurePolicy,
			StaticAddr:   &staticAddr,
		},
	}
}

func needUpdateRecord(curObj *lbcfapi.BackendRecord, expectObj *lbcfapi.BackendRecord) bool {
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

// IterateBackends runs handler on every BackendRecord in all and returns error if any error occurs
func IterateBackends(all []*lbcfapi.BackendRecord, handler func(*lbcfapi.BackendRecord) error) error {
	var errList []error
	for _, record := range all {
		if err := handler(record); err != nil {
			errList = append(errList, err)
		}
	}
	if len(errList) > 0 {
		return ErrorList(errList)
	}
	return nil
}

// FilterPods runs filter on every Pod in all and collects the Pod if filter returns true
func FilterPods(all []*v1.Pod, filter func(pod *v1.Pod) bool) []*v1.Pod {
	var ret []*v1.Pod
	for _, pod := range all {
		if filter(pod) {
			ret = append(ret, pod)
		}
	}
	return ret
}

// FilterBackendGroup runs filter on every BackendGroup in all and collects the BackendGroup if filter returns true
func FilterBackendGroup(all []*lbcfapi.BackendGroup, filter func(*lbcfapi.BackendGroup) bool) []*lbcfapi.BackendGroup {
	var ret []*lbcfapi.BackendGroup
	for _, group := range all {
		if filter(group) {
			ret = append(ret, group)
		}
	}
	return ret
}

// IsPodMatchBackendGroup returns true if pod is included in group
func IsPodMatchBackendGroup(group *lbcfapi.BackendGroup, pod *v1.Pod) bool {
	if group.Namespace != pod.Namespace {
		return false
	}
	if group.Spec.Pods == nil {
		return false
	}

	if group.Spec.Pods.ByLabel != nil {
		except := sets.NewString(group.Spec.Pods.ByLabel.Except...)
		if except.Has(pod.Name) {
			return false
		}
		selector := k8slabel.SelectorFromSet(k8slabel.Set(group.Spec.Pods.ByLabel.Selector))
		return selector.Matches(k8slabel.Set(pod.Labels))
	}
	included := sets.NewString(group.Spec.Pods.ByName...)
	return included.Has(pod.Name)
}

// IsLBMatchBackendGroup returns true if group is connected to lb
func IsLBMatchBackendGroup(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer) bool {
	if group.Namespace == lb.Namespace && group.Spec.LBName == lb.Name {
		return true
	}
	return false
}

// IsSvcMatchBackendGroup returns true if group is connected to lb
func IsSvcMatchBackendGroup(group *lbcfapi.BackendGroup, svc *v1.Service) bool {
	if group.Spec.Service == nil {
		return false
	}
	if group.Namespace == svc.Namespace && group.Spec.Service.Name == svc.Name {
		return true
	}
	return false
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
func CompareBackendRecords(expect []*lbcfapi.BackendRecord, have []*lbcfapi.BackendRecord) (needCreate, needUpdate, needDelete []*lbcfapi.BackendRecord) {
	expectedRecords := make(map[string]*lbcfapi.BackendRecord)
	for _, e := range expect {
		expectedRecords[e.Name] = e
	}
	haveRecords := make(map[string]*lbcfapi.BackendRecord)
	for _, h := range have {
		haveRecords[h.Name] = h
	}
	for k, v := range expectedRecords {
		cur, ok := haveRecords[k]
		if !ok {
			needCreate = append(needCreate, v)
			continue
		}
		if needUpdateRecord(cur, v) {
			update := cur.DeepCopy()
			update.Spec = v.Spec
			needUpdate = append(needUpdate, update)
		}
	}
	for k, v := range haveRecords {
		if _, ok := expectedRecords[k]; !ok {
			needDelete = append(needDelete, v)
		}
	}
	return
}

// BackendRegistered returns true if backend has been successfully registered
func BackendRegistered(backend *lbcfapi.BackendRecord) bool {
	cond := GetBackendRecordCondition(&backend.Status, lbcfapi.BackendRegistered)
	if cond != nil && cond.Status == lbcfapi.ConditionTrue {
		return true
	}
	return false
}

// BackendNeedEnsure returns true if webhook ensureBackend should be called on backend
func BackendNeedEnsure(backend *lbcfapi.BackendRecord) bool {
	if !BackendRegistered(backend) {
		return true
	}
	return backend.Spec.EnsurePolicy != nil && backend.Spec.EnsurePolicy.Policy == lbcfapi.PolicyAlways
}

// SyncResult stores result for sync method of controllers
type SyncResult struct {
	err error

	operationFailed   bool
	asyncOperation    bool
	periodicOperation bool

	minRetryDelay     time.Duration
	minReEnsurePeriod time.Duration
}

// SuccResult returns a new SyncResult that call IsSucc() on it will return true
func SuccResult() *SyncResult {
	return &SyncResult{}
}

// ErrorResult returns a new SyncResult that call IsError() on it will return true
func ErrorResult(err error) *SyncResult {
	return &SyncResult{err: err}
}

// FailResult returns a new SyncResult that call IsFailed() on it will return true
func FailResult(delay time.Duration) *SyncResult {
	return &SyncResult{operationFailed: true, minRetryDelay: delay}
}

// AsyncResult returns a new SyncResult that call IsPeriodic() on it will return true
func AsyncResult(period time.Duration) *SyncResult {
	return &SyncResult{asyncOperation: true, minReEnsurePeriod: period}
}

// PeriodicResult returns a new SyncResult that call IsPeriodic() on it will return true
func PeriodicResult(period time.Duration) *SyncResult {
	return &SyncResult{periodicOperation: true, minReEnsurePeriod: period}
}

// IsSucc indicates the operation is successfully finished
func (s *SyncResult) IsSucc() bool {
	return !s.IsError() && !s.IsFailed() && !s.IsRunning()
}

// IsError indicates an error occurred during operation
func (s *SyncResult) IsError() bool {
	return s.err != nil
}

// IsFailed indicates no error occured during operation, but the operation failed
func (s *SyncResult) IsFailed() bool {
	return s.operationFailed
}

// IsRunning indicates the operation is still in progress
func (s *SyncResult) IsRunning() bool {
	return s.asyncOperation
}

// IsPeriodic indicates the operation successfully finished and should be called periodically
func (s *SyncResult) IsPeriodic() bool {
	return s.periodicOperation
}

// GetError returns the error stored in SyncResult
func (s *SyncResult) GetError() error {
	return s.err
}

// GetRetryDelay returns in how long time the operation should be retried
func (s *SyncResult) GetRetryDelay() time.Duration {
	return s.minRetryDelay
}

// GetReEnsurePeriodic returns in how long time the operation should be taken again
func (s *SyncResult) GetReEnsurePeriodic() time.Duration {
	return s.minReEnsurePeriod
}

var (
	firstFirnalizerPatchTemplate      = `[{"op":"add","path":"/metadata/finalizers","value":["{{ .Finalizer }}"]}]`
	additionalFirnalizerPatchTemplate = `[{"op":"add","path":"/metadata/finalizers/-","value":"{{ .Finalizer }}"}]`
)

// MakeFinalizerPatch returns a patch that is used in MutatingAdmissionWebhook to add a finalizer into ObjectMeta.Finalizers
func MakeFinalizerPatch(isFirst bool, finalizer string) []byte {
	tmpStr := firstFirnalizerPatchTemplate
	if !isFirst {
		tmpStr = additionalFirnalizerPatchTemplate
	}
	t := template.Must(template.New("patch").Parse(tmpStr))

	wrapper := struct {
		Finalizer string
	}{
		Finalizer: finalizer,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, wrapper); err != nil {
		klog.Errorf("make finalizer patch failed: %v", err)
		return nil
	}
	return buf.Bytes()
}

var (
	addLabelPatchTemplate     = `[{"op":"add","path":"/metadata/labels","value": { "{{ .Key }}": "{{ .Value }}"} }]`
	replaceLabelPatchTemplate = `[{"op":"replace","path":"/metadata/labels/{{ .Key }}","value": "{{ .Value }}" }]`
)

// MakeLabelPatch returns a patch that is used in MutatingAdmissionWebhook to add a label into ObjectMeta.Labels
func MakeLabelPatch(isReplace bool, key string, value string) []byte {
	tmpStr := addLabelPatchTemplate
	if isReplace {
		tmpStr = replaceLabelPatchTemplate
		key = strings.ReplaceAll(key, "~", "~0")
		key = strings.ReplaceAll(key, "/", "~1")
	}
	t := template.Must(template.New("patch").Parse(tmpStr))

	wrapper := struct {
		Key   string
		Value string
	}{
		Key:   key,
		Value: value,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, wrapper); err != nil {
		klog.Errorf("make label patch failed: %v", err)
		return nil
	}
	return buf.Bytes()
}

// DetermineNeededBackendGroupUpdates compares oldGroups with groups, and returns BackendGroups that should be
func DetermineNeededBackendGroupUpdates(oldGroups, groups sets.String, podStatusChanged bool) sets.String {
	if podStatusChanged {
		groups = groups.Union(oldGroups)
	} else {
		groups = groups.Difference(oldGroups).Union(oldGroups.Difference(groups))
	}
	return groups
}

// GetPortSelectorProto returns protocol of the selected port
func GetPortSelectorProto(selector lbcfapi.PortSelector) string {
	return selector.Protocol
}
