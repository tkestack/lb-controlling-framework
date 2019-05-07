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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"text/template"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"github.com/parnurzeal/gorequest"
	"golang.org/x/time/rate"
	"k8s.io/api/core/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/api/v1/pod"
)

const (
	DefaultRetryInterval = 10 * time.Second
	DefaultEnsurePeriod  = 1 * time.Minute
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

func NewIntervalRateLimitingQueue(rateLimiter workqueue.RateLimiter, name string, defaultDelay time.Duration) IntervalRateLimitingInterface {
	return &IntervalRateLimitingQueue{
		defaultRetryDelay: defaultDelay,
		DelayingInterface: workqueue.NewNamedDelayingQueue(name),
		rateLimiter:       rateLimiter,
	}
}

type IntervalRateLimitingQueue struct {
	defaultRetryDelay time.Duration

	workqueue.DelayingInterface

	rateLimiter workqueue.RateLimiter
}

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

func PodAvailable(obj *v1.Pod) bool {
	return obj.Status.PodIP != "" && obj.DeletionTimestamp == nil && pod.IsPodReady(obj)
}

func LBCreated(lb *lbcfapi.LoadBalancer) bool {
	condition := getLBCondition(&lb.Status, lbcfapi.LBCreated)
	if condition == nil {
		return false
	}
	return condition.Status == lbcfapi.ConditionTrue
}

func LBNeedEnsure(lb *lbcfapi.LoadBalancer) bool {
	condition := getLBCondition(&lb.Status, lbcfapi.LBEnsured)
	return condition == nil || condition.Status != lbcfapi.ConditionTrue
}

func getLBCondition(status *lbcfapi.LoadBalancerStatus, conditionType lbcfapi.LoadBalancerConditionType) *lbcfapi.LoadBalancerCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}
	return nil
}

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

type BackendType string

const (
	TypeService BackendType = "Service"
	TypePod     BackendType = "Pod"
	TypeStatic  BackendType = "Static"
	TypeUnknown BackendType = "Unknown"
)

func GetBackendType(bg *lbcfapi.BackendGroup) BackendType {
	if bg.Spec.Pods != nil {
		return TypePod
	} else if bg.Spec.Service != nil {
		return TypeService
	} else if len(bg.Spec.Static) > 0 {
		return TypeStatic
	}
	return TypeUnknown
}

func GetDriverNamespace(driverName string, defaultNamespace string) string {
	if strings.HasPrefix(driverName, lbcfapi.SystemDriverPrefix) {
		return "kube-system"
	}
	return defaultNamespace
}

func IsDriverDraining(labels map[string]string) bool {
	if v, ok := labels[lbcfapi.DriverDrainingLabel]; !ok || strings.ToUpper(v) != "TRUE" {
		return false
	}
	return true
}

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

type PodProvider interface {
	Get(namespace string, name string) (*v1.Pod, error)
	Select(labels map[string]string) ([]*v1.Pod, error)
}

func NewPodProvider(lister corev1.PodLister) PodProvider {
	return &PodProviderImpl{
		lister: lister,
	}
}

type PodProviderImpl struct {
	lister corev1.PodLister
}

func (p *PodProviderImpl) Get(namespace string, name string) (*v1.Pod, error) {
	return p.lister.Pods(namespace).Get(name)
}

func (p *PodProviderImpl) Select(labels map[string]string) ([]*v1.Pod, error) {
	return p.lister.List(k8slabel.SelectorFromSet(k8slabel.Set(labels)))
}

func HasFinalizer(all []string, expect string) bool {
	for i := range all {
		if all[i] == expect {
			return true
		}
	}
	return false
}

func RemoveFinalizer(all []string, toDelete string) []string {
	var ret []string
	for i := range all {
		if all[i] != toDelete {
			ret = append(ret, all[i])
		}
	}
	return ret
}

func NamespacedNameKeyFunc(namespace, name string) string {
	if len(namespace) > 0 {
		return namespace + "/" + name

	}
	return name
}

func GetDuration(cfg *lbcfapi.Duration, defaultValue time.Duration) time.Duration {
	if cfg == nil {
		return defaultValue
	}
	return cfg.Duration
}

func MapEqual(a map[string]string, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func ResyncPolicyEqual(a *lbcfapi.ResyncPolicyConfig, b *lbcfapi.ResyncPolicyConfig) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Policy != b.Policy {
		return false
	}
	if a.Policy == lbcfapi.PolicyAlways {
		if a.MinPeriod == b.MinPeriod {
			return true
		}
		if a.MinPeriod == nil || b.MinPeriod == nil {
			return false
		}
		if a.MinPeriod.Duration.Nanoseconds() != b.MinPeriod.Duration.Nanoseconds() {
			return false
		}
		return true
	}
	return true
}

type ErrorList []error

func (e ErrorList) Error() string {
	var msg []string
	for i, err := range e {
		msg = append(msg, fmt.Sprintf("%d: %v", i+1, err))
	}
	return strings.Join(msg, "\n")
}

const (
	DefaultWebhookTimeout = 10 * time.Second
)

func CallWebhook(driver *lbcfapi.LoadBalancerDriver, webHookName string, payload interface{}, rsp interface{}) error {
	u, err := url.Parse(driver.Spec.Url)
	if err != nil {
		e := fmt.Errorf("invalid url: %v", err)
		klog.Errorf("callwebhook failed: %v. driver: %s, webhookName: %s", e, driver.Name, webHookName)
		return e
	}
	u.Path = path.Join(webHookName)
	timeout := DefaultWebhookTimeout
	for _, h := range driver.Spec.Webhooks {
		if h.Name == webHookName {
			if h.Timeout != nil {
				timeout = h.Timeout.Duration
			}
			break
		}
	}
	request := gorequest.New().Timeout(timeout).Post(u.String()).Send(payload)
	debugInfo, _ := request.AsCurlCommand()
	klog.V(3).Infof("callwebhook, %s", debugInfo)

	response, body, errs := request.EndBytes()
	if len(errs) > 0 {
		e := fmt.Errorf("webhook err: %v", errs)
		klog.Errorf("callwebhook failed: %v. url: %s", e, u.String())
		return e
	}
	if response.StatusCode != http.StatusOK {
		e := fmt.Errorf("http status code: %d, body: %s", response.StatusCode, body)
		klog.Errorf("callwebhook failed: %v. url: %s", e, u.String())
		return e
	}
	if err := json.Unmarshal(body, rsp); err != nil {
		e := fmt.Errorf("decode webhook response err: %v, raw: %s", err, body)
		klog.Errorf("callwebhook failed: %v. url: %s", e, u.String())
		return e
	}
	return nil
}

func MakeBackendName(lbName, groupName, podName string, port lbcfapi.PortSelector) string {
	protocol := "TCP"
	if port.Protocol != nil {
		protocol = strings.ToUpper(*port.Protocol)
	}
	raw := fmt.Sprintf("%s_%s_%s_%d_%+v", lbName, groupName, podName, port.PortNumber, protocol)
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", h)
}

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

func needUpdateRecord(curObj *lbcfapi.BackendRecord, expectObj *lbcfapi.BackendRecord) bool {
	if !MapEqual(curObj.Spec.LBAttributes, expectObj.Spec.LBAttributes) {
		return true
	}
	if !MapEqual(curObj.Spec.Parameters, expectObj.Spec.Parameters) {
		return true
	}
	if !ResyncPolicyEqual(curObj.Spec.ResyncPolicy, expectObj.Spec.ResyncPolicy) {
		return true
	}
	return false
}

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

func FilterPods(all []*v1.Pod, filter func(pod *v1.Pod) bool) []*v1.Pod {
	var ret []*v1.Pod
	for _, pod := range all {
		if filter(pod) {
			ret = append(ret, pod)
		}
	}
	return ret
}

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

func IsLBMatchBackendGroup(group *lbcfapi.BackendGroup, lb *lbcfapi.LoadBalancer) bool {
	if group.Namespace == lb.Namespace && group.Spec.LBName == lb.Name {
		return true
	}
	return false
}

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
			needUpdate = append(needUpdate, v)
		}
	}
	for k, v := range haveRecords {
		if _, ok := expectedRecords[k]; !ok {
			needDelete = append(needDelete, v)
		}
	}
	return
}

func BackendNeedEnsure(backend *lbcfapi.BackendRecord) bool {
	if !backendIsRegistered(backend) {
		return true
	}
	return backend.Spec.ResyncPolicy != nil && backend.Spec.ResyncPolicy.Policy == lbcfapi.PolicyAlways
}

func backendIsRegistered(backend *lbcfapi.BackendRecord) bool {
	for _, cond := range backend.Status.Conditions {
		if cond.Type == lbcfapi.BackendRegistered {
			if cond.Status == lbcfapi.ConditionTrue {
				return true
			}
			return false
		}
	}
	return false
}

type SyncFunc func(string) *SyncResult

type SyncResult struct {
	err error

	operationFailed   bool
	asyncOperation    bool
	periodicOperation bool

	minRetryDelay   time.Duration
	minResyncPeriod time.Duration
}

func SuccResult() *SyncResult {
	return &SyncResult{}
}

func ErrorResult(err error) *SyncResult {
	return &SyncResult{err: err}
}

func FailResult(delay time.Duration) *SyncResult {
	return &SyncResult{operationFailed: true, minRetryDelay: delay}
}

func AsyncResult(period time.Duration) *SyncResult {
	return &SyncResult{asyncOperation: true, minResyncPeriod: period}
}

func PeriodicResult(period time.Duration) *SyncResult {
	return &SyncResult{periodicOperation: true, minResyncPeriod: period}
}

func (s *SyncResult) IsError() bool {
	return s.err != nil
}

func (s *SyncResult) IsFailed() bool {
	return s.operationFailed
}

func (s *SyncResult) IsAsync() bool {
	return s.asyncOperation
}

func (s *SyncResult) IsPeriodic() bool {
	return s.periodicOperation
}

func (s *SyncResult) GetError() error {
	return s.err
}

func (s *SyncResult) GetRetryDelay() time.Duration {
	return s.minRetryDelay
}

func (s *SyncResult) GetResyncPeriodic() time.Duration {
	return s.minResyncPeriod
}

var firstFirnalizerPatchTemplate = `[{"op":"add","path":"/metadata/finalizers","value":["{{ .Finalizer }}"]}]`
var additionalFirnalizerPatchTemplate = `[{"op":"add","path":"/metadata/finalizers/-","value":"{{ .Finalizer }}"}]`

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
