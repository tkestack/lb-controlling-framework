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
	}else if err != nil {
		return err, nil
	}
	driver, err := c.driverProvider.getDriver(lb.Spec.LBDriver, getDriverNamespace(lb.Spec.LBDriver, lb.Namespace))
	if err != nil {
		return err, nil
	}

	// delete load balancer
	if lb.DeletionTimestamp != nil {
		needDelete := false
		for _, f := range lb.Finalizers {
			if f == "lbcf.tke.cloud.tencent.com/delete-load-loadbalancer" {
				needDelete = true
				break
			}
		}
		if !needDelete {
			return nil, nil
		}
		req := &DeleteLoadBalancerRequest{
			RequestForRetryHooks: RequestForRetryHooks{
				RecordID: string(lb.UID),
				RetryID:  string(uuid.NewUUID()),
			},
			Attributes: lb.Spec.Attributes,
		}
		rsp, err := callDeleteLoadBalancer(driver, req)
		if err != nil {
			return err, nil
		}
		switch rsp.Status {
		case StatusSucc:
			// remove finalizer
			var fin []string
			for _, f := range lb.Finalizers {
				if f != "lbcf.tke.cloud.tencent.com/delete-load-loadbalancer" {
					fin = append(fin, f)
				}
				lbCpy := lb.DeepCopy()
				lbCpy.Finalizers = fin
				if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).Update(lbCpy); err != nil {
					return err, nil
				}
			}
		case StatusFail:
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		case StatusRunning:
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
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
		rsp, err := callEnsureLoadBalancer(driver, req)
		if err != nil {
			return err, nil
		}
		switch rsp.Status {
		case StatusSucc:
			lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
				Type:               lbcfapi.LBEnsured,
				Status:             lbcfapi.ConditionTrue,
				LastTransitionTime: v1.Now(),
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
		case StatusFail:
			if rsp.Msg != "" {
				lb.Status = addLBCondition(&lb.Status, lbcfapi.LoadBalancerCondition{
					Type:               lbcfapi.LBEnsured,
					Status:             lbcfapi.ConditionFalse,
					LastTransitionTime: v1.Now(),
					Reason:             "EnsureFailed",
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
				Type:               lbcfapi.LBEnsured,
				Status:             lbcfapi.ConditionFalse,
				LastTransitionTime: v1.Now(),
				Reason:             "OperationFailed",
				Message:            rsp.Msg,
			})
			if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
				return err, nil
			}
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		}
		if rsp.Status == StatusFail {
			delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
			return fmt.Errorf(rsp.Msg), &delay
		}

		// TODO: update LoadBalancer.status.condition
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
	rsp, err := callCreateLoadBalancer(driver, req)
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
				Reason:             "OperationFailed",
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
			Reason:             "InProgress",
			Message:            rsp.Msg,
		})
		if _, err := c.lbcfClient.LbcfV1beta1().LoadBalancers(namespace).UpdateStatus(lb); err != nil {
			return err, nil
		}
		delay := calculateRetryInterval(DefaultWebhookTimeout, rsp.RetryIntervalInSeconds)
		return fmt.Errorf(rsp.Msg), &delay
	}
	// TODO: handle status update as part of resource
	return nil, nil
}
