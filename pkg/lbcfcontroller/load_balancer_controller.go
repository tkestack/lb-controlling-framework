package lbcfcontroller

import (
	"fmt"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"k8s.io/apimachinery/pkg/util/uuid"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

func NewLoadBalancerController(lister v1beta1.LoadBalancerLister, driverProvider DriverProvider) *LoadBalancerController {
	return &LoadBalancerController{
		lister: lister,
	}
}

type LoadBalancerController struct {
	lister v1beta1.LoadBalancerLister
	driverProvider DriverProvider
}

func (c *LoadBalancerController) syncLB(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	lb, err := c.lister.LoadBalancers(namespace).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if lb.DeletionTimestamp != nil {
		// TODO: callWebhook driver.deleteLoadBalancer

		// TODO: remove finalizer
	}

	if !lbCreated(lb) {
		// TODO: callWebhook driver.createLoadBalancer

		// TODO: update LoadBalancer.status.condition
		return nil
	}

	driver, err := c.driverProvider.getDriver(lb.Spec.LBDriver, getDriverNamespace(lb))
	if err != nil{
		return err
	}
	req := &CreateLoadBalancerRequest{
		RequestForRetryHooks: RequestForRetryHooks{
			RecordID: string(lb.UID),
			RetryID: string(uuid.NewUUID()),
		},
		LBSpec: lb.Spec.LBSpec,
		Attributes: lb.Spec.Attributes,
	}
	rsp, err := callCreateLoadBalancer(driver, req)
	if err != nil{
		return err
	}else if rsp.Status == StatusFail{
		return fmt.Errorf(rsp.ErrMsg)
	}

	// TODO: handle status = running
	return nil

}


