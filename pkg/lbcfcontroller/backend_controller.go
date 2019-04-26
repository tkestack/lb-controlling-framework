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
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/cache"
)

type BackendProvider interface {
	listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error)
}

func NewBackendController(client *lbcfclient.Clientset, bgLister v1beta1.BackendGroupLister, brLister v1beta1.BackendRecordLister, driverProvider DriverProvider, lbProvider LoadBalancerProvider, podProvider PodProvider) *BackendController {
	return &BackendController{
		client:         client,
		bgLister:       bgLister,
		brLister:       brLister,
		driverProvider: driverProvider,
		lbProvider:     lbProvider,
		podProvider:    podProvider,
		//pendingRecords: cache.NewStore(recordIndex),
	}
}

type BackendController struct {
	client *lbcfclient.Clientset

	bgLister v1beta1.BackendGroupLister
	brLister v1beta1.BackendRecordLister

	driverProvider DriverProvider
	lbProvider     LoadBalancerProvider
	podProvider    PodProvider

	//pendingRecords cache.Store
}

func (c *BackendController) syncBackendGroup(key string) (error, *time.Duration) {
	return nil, nil
}

func (c *BackendController) syncBackendRecord(key string) (error, *time.Duration) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err, nil
	}
	backend, err := c.brLister.BackendRecords(namespace).Get(name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return err, nil
	}

	if backend.DeletionTimestamp != nil {
		if !hasFinalizer(backend.Finalizers, DeregisterBackendFinalizer) {
			// leave the job to k8s gc
			return nil, nil
		}

		if backend.Status.BackendAddr == "" {
			// remove the finalizer so that k8s gc will delete this object
			cpy := backend.DeepCopy()
			cpy.Finalizers = removeFinalizer(cpy.Finalizers, DeregisterBackendFinalizer)
			if _, err := c.client.LbcfV1beta1().BackendRecords(namespace).Update(cpy); err != nil {
				return err, nil
			}
			return nil, nil
		}

		// deregister backend
		driver, exist := c.driverProvider.getDriver(getDriverNamespace(backend.Spec.LBDriver, backend.Namespace), backend.Spec.LBDriver)
		if !exist {
			return fmt.Errorf("driver %q not found for BackendRecord %s", backend.Spec.LBDriver, backend.Name), nil
		}
		req := &DeregisterBackendRequest{
			LBInfo: backend.Spec.LBInfo,
			Backends:[]BackendDereg{
				{
					RequestForRetryHooks: RequestForRetryHooks{
						RecordID: string(backend.UID),
						RetryID: string(uuid.NewUUID()),
					},
					BackendAddr: backend.Status.BackendAddr,
					Parameters: backend.Spec.Parameters,
					InjectedInfo: backend.Status.InjectedInfo,
				},
			},
		}
		rsp, err := driver.CallDeregisterBackend(req)
		if err != nil{
			return err, nil
		}
		foundResult := false
		for _, result := range rsp.Results{
			if result.RecordID != string(backend.UID){
				continue
			}
			foundResult = true
			switch result.Status {
			case StatusSucc:
				cpy := backend.DeepCopy()
				cpy.Status = addBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
					Type:               lbcfapi.BackendRegistered,
					Status:             lbcfapi.ConditionFalse,
					LastTransitionTime: v1.Now(),
				})
				cpy.Status.BackendAddr = ""
				if _, err := c.client.LbcfV1beta1().BackendRecords(namespace).UpdateStatus(cpy); err != nil {
					return err, nil
				}
				return nil, nil
			case StatusFail:
				delay := calculateRetryInterval(DefaultWebhookTimeout, result.RetryIntervalInSeconds)
				return fmt.Errorf(result.Msg), &delay
			case StatusRunning:
				delay := calculateRetryInterval(DefaultWebhookTimeout, result.RetryIntervalInSeconds)
				return fmt.Errorf(result.Msg), &delay
			default:
				return fmt.Errorf("unknown status %q", result.Status), nil
			}
			break
		}
		if !foundResult{
			return fmt.Errorf("invalid response, no result found for backend %s", backend.Name), nil
		}
		return nil, nil
	}

	if backend.Status.BackendAddr == "" {
		driver, exist := c.driverProvider.getDriver(getDriverNamespace(backend.Spec.LBDriver, backend.Namespace), backend.Spec.LBDriver)
		if !exist {
			return fmt.Errorf("driver %q not found for BackendRecord %s", backend.Spec.LBDriver, backend.Name), nil
		}

		// generate backend address for pod backend
		if backend.Spec.PodBackendInfo != nil {
			pod, err := c.podProvider.GetPod(backend.Spec.PodBackendInfo.Name, backend.Namespace)
			if err != nil {
				return err, nil
			}
			req := &GenerateBackendAddrRequest{
				RequestForRetryHooks: RequestForRetryHooks{
					RecordID: string(backend.UID),
					RetryID:  string(uuid.NewUUID()),
				},
				PodBackend: &PodBackendInGenerateAddrRequest{
					Pod:  *pod,
					Port: containerPortToK8sContainerPort(backend.Spec.PodBackendInfo.Port),
				},
			}
			rsp, err := driver.CallGenerateBackendAddr(req)
			if err != nil {
				return err, nil
			}
			switch rsp.Status {
			case StatusSucc:
				cpy := backend.DeepCopy()
				cpy.Status.BackendAddr = rsp.BackendAddr
				if _, err := c.client.LbcfV1beta1().BackendRecords(namespace).UpdateStatus(cpy); err != nil {
					return err, nil
				}
				return nil, nil
			case StatusFail:
				delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
				return fmt.Errorf(rsp.Msg), &delay
			case StatusRunning:
				delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
				return fmt.Errorf(rsp.Msg), &delay
			}
			return nil, nil
		}
	}

	// ensure backend
	driver, exist := c.driverProvider.getDriver(getDriverNamespace(backend.Spec.LBDriver, backend.Namespace), backend.Spec.LBDriver)
	if !exist {
		return fmt.Errorf("driver %q not found for BackendRecord %s", backend.Spec.LBDriver, backend.Name), nil
	}

	req := &EnsureBackendRequest{
		LBInfo: backend.Spec.LBInfo,
		Backends:[]BackendReg{
			{
				RequestForRetryHooks: RequestForRetryHooks{
					RecordID: string(backend.UID),
					RetryID: string(uuid.NewUUID()),
				},
				BackendAddr: backend.Status.BackendAddr,
				Parameters: backend.Spec.Parameters,
				InjectedInfo: backend.Status.InjectedInfo,
			},
		},
	}
	rsp, err := driver.CallEnsureBackend(req)
	if err != nil{
		return err, nil
	}
	foundResult := false
	for _, result := range rsp.Results{
		if result.RecordID != string(backend.UID){
			continue
		}
		foundResult = true
		switch result.Status {
		case StatusSucc:
			cpy := backend.DeepCopy()
			cpy.Status = addBackendCondition(&cpy.Status, lbcfapi.BackendRecordCondition{
				Type:               lbcfapi.BackendRegistered,
				Status:             lbcfapi.ConditionTrue,
				LastTransitionTime: v1.Now(),
			})
			if _, err := c.client.LbcfV1beta1().BackendRecords(namespace).UpdateStatus(cpy); err != nil {
				return err, nil
			}
			return nil, nil
		case StatusFail:
			delay := calculateRetryInterval(DefaultWebhookTimeout, result.RetryIntervalInSeconds)
			return fmt.Errorf(result.Msg), &delay
		case StatusRunning:
			delay := calculateRetryInterval(DefaultWebhookTimeout, result.RetryIntervalInSeconds)
			return fmt.Errorf(result.Msg), &delay
		default:
			return fmt.Errorf("unknown status %q", result.Status), nil
		}
		break
	}
	if !foundResult{
		return fmt.Errorf("invalid response, no result found for backend %s", backend.Name), nil
	}
	return nil, nil
}

func (c *BackendController) listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error) {
	recordList, err := c.brLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.BackendRecord
	for _, r := range recordList {
		if driverNamespace != "kube-system" && r.Namespace != driverNamespace {
			continue
		}
		if r.Spec.LBDriver == driverName {
			ret = append(ret, r)
		}
	}
	return ret, nil
}
