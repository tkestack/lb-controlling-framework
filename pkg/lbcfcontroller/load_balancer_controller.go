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
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/cache"
)

type LoadBalancerProvider interface {
	getLoadBalancer(name, namespace string) (*lbcfapi.LoadBalancer, error)
	listLoadBalancerByDriver(driverName string, driverNamespace string) ([]*lbcfapi.LoadBalancer, error)
}

func NewLoadBalancerController(client *lbcfclient.Clientset, lister v1beta1.LoadBalancerLister, driverProvider DriverProvider) *LoadBalancerController {
	return &LoadBalancerController{
		lbcfClient:     client,
		lister:         lister,
		driverProvider: driverProvider,
	}
}

type LoadBalancerController struct {
	lbcfClient     *lbcfclient.Clientset
	lister         v1beta1.LoadBalancerLister
	driverProvider DriverProvider
}

func (c *LoadBalancerController) getLoadBalancer(name, namespace string) (*lbcfapi.LoadBalancer, error) {
	return c.lister.LoadBalancers(namespace).Get(name)
}

func (c *LoadBalancerController) listLoadBalancerByDriver(driverName string, driverNamespace string) ([]*lbcfapi.LoadBalancer, error) {
	lbList, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.LoadBalancer
	for _, lb := range lbList {
		if driverNamespace != "kube-system" && lb.Namespace != driverNamespace {
			continue
		}
		if lb.Spec.LBDriver == driverName {
			ret = append(ret, lb)
		}
	}
	return ret, nil
}

func (c *LoadBalancerController) syncLB(key string) (error, *time.Duration) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err, nil
	}
	lb, err := c.lister.LoadBalancers(namespace).Get(name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return err, nil
	}

	driver, exist := c.driverProvider.getDriver(getDriverNamespace(lb.Spec.LBDriver, lb.Namespace), lb.Spec.LBDriver)
	if !exist {
		return fmt.Errorf("driver %q not found for LoadBalancer %s", lb.Spec.LBDriver, lb.Name), nil
	}

	// delete load balancer
	if lb.DeletionTimestamp != nil {
		if !hasFinalizer(lb.Finalizers, DeleteLBFinalizer) {
			return nil, nil
		}
		req := &DeleteLoadBalancerRequest{
			RequestForRetryHooks: RequestForRetryHooks{
				RecordID: string(lb.UID),
				RetryID:  string(uuid.NewUUID()),
			},
			Attributes: lb.Spec.Attributes,
		}
		rsp, err := driver.CallDeleteLoadBalancer(req)
		if err != nil {
			return err, nil
		}
		switch rsp.Status {
		case StatusSucc:
			lbCpy := lb.DeepCopy()
			lbCpy.Finalizers = removeFinalizer(lbCpy.Finalizers, DeleteLBFinalizer)
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).Update(lbCpy); err != nil {
				return err, nil
			}
		case StatusFail:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBDeleted,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             lbcfapi.ReasonOperationFailed.String(),
				Message:            rsp.Msg,
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		case StatusRunning:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBDeleted,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             lbcfapi.ReasonOperationInProgress.String(),
				Message:            rsp.Msg,
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		default:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBDeleted,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             lbcfapi.ReasonInvalidResponse.String(),
				Message:            fmt.Sprintf("unknown status %q", rsp.Status),
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
			return fmt.Errorf("unknown status %q", rsp.Status), nil
		}
	}

	// ensure load balancer
	if lbCreated(lb) {
		req := &EnsureLoadBalancerRequest{
			RequestForRetryHooks: RequestForRetryHooks{
				RecordID: string(lb.UID),
				RetryID:  string(uuid.NewUUID()),
			},
			Attributes: lb.Spec.Attributes,
		}
		rsp, err := driver.CallEnsureLoadBalancer(req)
		if err != nil {
			return err, nil
		}
		switch rsp.Status {
		case StatusSucc:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBSatisfied,
				Status:             lbcfapi.ConditionTrue,
				LastTransitionTime: v1.Now(),
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
		case StatusFail:
			if rsp.Msg != "" {
				lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
					Type:               lbcfapi.LBSatisfied,
					Status:             lbcfapi.ConditionFalse,
					LastTransitionTime: v1.Now(),
					Reason:             lbcfapi.ReasonOperationFailed.String(),
					Message:            rsp.Msg,
				})
				if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
					return err, nil
				}
			}
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		case StatusRunning:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBSatisfied,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             lbcfapi.ReasonOperationInProgress.String(),
				Message:            rsp.Msg,
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		default:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBSatisfied,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             lbcfapi.ReasonInvalidResponse.String(),
				Message:            fmt.Sprintf("unknown status %q", rsp.Status),
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
			return fmt.Errorf("unknown status %q", rsp.Status), nil
		}
		return nil, nil
	}

	// create load balancer
	req := &CreateLoadBalancerRequest{
		RequestForRetryHooks: RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		LBSpec:     lb.Spec.LBSpec,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := driver.CallCreateLoadBalancer(req)
	if err != nil {
		return err, nil
	}
	switch rsp.Status {
	case StatusSucc:
		lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionTrue,
			LastTransitionTime: v1.Now(),
		})
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
			return err, nil
		}
	case StatusFail:
		if rsp.Msg != "" {
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBCreated,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             lbcfapi.ReasonOperationFailed.String(),
				Message:            rsp.Msg,
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
		}
		delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
		return fmt.Errorf(rsp.Msg), &delay
	case StatusRunning:
		lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionFalse,
			LastTransitionTime: v1.Now(),
			Reason:             lbcfapi.ReasonOperationInProgress.String(),
			Message:            rsp.Msg,
		})
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
			return err, nil
		}
		delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
		return fmt.Errorf(rsp.Msg), &delay
	default:
		lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
			Type:               lbcfapi.LBCreated,
			Status:             lbcfapi.ConditionFalse,
			LastTransitionTime: v1.Now(),
			Reason:             lbcfapi.ReasonInvalidResponse.String(),
			Message:            fmt.Sprintf("unknown status %q", rsp.Status),
		})
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
			return err, nil
		}
		return fmt.Errorf("unknown status %q", rsp.Status), nil
	}
	// TODO: handle status update as part of resource
	return nil, nil
}
