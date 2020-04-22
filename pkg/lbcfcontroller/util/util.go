/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */

package util

import (
	"crypto/md5"
	"fmt"
	"reflect"
	"strings"
	"time"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabel "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	// DefaultRetryInterval is the default minimum delay for retries
	DefaultRetryInterval = 10 * time.Second

	// DefaultEnsurePeriod is the default minimum interval for ensureLoadBalancer and ensureBackendRecord
	DefaultEnsurePeriod = 1 * time.Minute
)

// IsPodReady returns true if a pod is ready; false otherwise.
func IsPodReady(pod *v1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

// IsPodReady returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status v1.PodStatus) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && condition.Status == v1.ConditionTrue
}

// Extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func GetPodReadyCondition(status v1.PodStatus) *v1.PodCondition {
	_, condition := GetPodCondition(&status, v1.PodReady)
	return condition
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

// PodAvailable indicates the given pod is ready to bind to load balancers
func PodAvailable(obj *v1.Pod) bool {
	return obj.Status.PodIP != "" && obj.DeletionTimestamp == nil && IsPodReady(obj)
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
	raw := fmt.Sprintf("%s_%s_%s_%d_%s", lbName, groupName, podUID, port.PortNumber, port.Protocol)
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
func MakeBackendLabels(driverName, lbName, groupName, svcName, podName string) map[string]string {
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
	return ret
}

// ConstructPodBackendRecord constructs a new BackendRecord
func ConstructPodBackendRecord(lb *lbcfapi.LoadBalancer, group *lbcfapi.BackendGroup, pod *v1.Pod) *lbcfapi.BackendRecord {
	valueTrue := true
	return &lbcfapi.BackendRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakePodBackendName(lb.Name, group.Name, pod.UID, group.Spec.Pods.Port),
			Namespace: group.Namespace,
			Labels:    MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, "", pod.Name),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         lbcfapi.APIVersion,
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
		if svcPort.Port == wantedPort.PortNumber && string(svcPort.Protocol) == wantedPort.Protocol {
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
			Labels:    MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, svc.Name, ""),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         lbcfapi.APIVersion,
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
			Labels:    MakeBackendLabels(lb.Spec.LBDriver, lb.Name, group.Name, "", ""),
			Finalizers: []string{
				lbcfapi.FinalizerDeregisterBackend,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         lbcfapi.APIVersion,
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

// DetermineNeededBackendGroupUpdates compares oldGroups with groups, and returns BackendGroups that should be
func DetermineNeededBackendGroupUpdates(oldGroups, groups sets.String, podStatusChanged bool) sets.String {
	if podStatusChanged {
		groups = groups.Union(oldGroups)
	} else {
		groups = groups.Difference(oldGroups).Union(oldGroups.Difference(groups))
	}
	return groups
}

// NeedEnqueueLB determines if the given LoadBalancer should be enqueue
func NeedEnqueueLB(old *lbcfapi.LoadBalancer, cur *lbcfapi.LoadBalancer) bool {
	if old.DeletionTimestamp == nil && cur.DeletionTimestamp != nil {
		return true
	}
	if old.Generation != cur.Generation {
		return true
	}
	oldCreated := LBCreated(old)
	curCreated := LBCreated(cur)
	curAsynced := LBEnsured(cur)
	if !oldCreated && curCreated && !curAsynced {
		return true
	}
	return false
}

// NeedEnqueueBackend determines if the given BackendRecord should be enqueue
func NeedEnqueueBackend(old *lbcfapi.BackendRecord, cur *lbcfapi.BackendRecord) bool {
	if old.DeletionTimestamp == nil && cur.DeletionTimestamp != nil {
		return true
	}
	if old.Generation != cur.Generation {
		return true
	}
	if old.Status.BackendAddr != cur.Status.BackendAddr {
		return true
	}
	return false
}

// NeedPeriodicEnsure tests if ensurePolicy is on
func NeedPeriodicEnsure(cfg *lbcfapi.EnsurePolicyConfig, deleting bool) bool {
	if deleting {
		return false
	}
	if cfg != nil && cfg.Policy == lbcfapi.PolicyAlways {
		return true
	}
	return false
}
