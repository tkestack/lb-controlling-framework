package lbcfcontroller

import (
	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)


type DriverProvider interface {
	getDriver(name string, namespace string) (*lbcfapi.LoadBalancerDriver, error)
}

func NewDriverController(client *lbcfclient.Clientset, lister v1beta1.LoadBalancerDriverLister) *DriverController {
	return &DriverController{
		lbcfClient: client,
		lister: lister,
	}
}

type DriverController struct {
	lbcfClient *lbcfclient.Clientset
	lister v1beta1.LoadBalancerDriverLister
}

func (c *DriverController) getDriver(name, namespace string) (*lbcfapi.LoadBalancerDriver, error) {
	return c.lister.LoadBalancerDrivers(name).Get(namespace)
}

func (c *DriverController) syncDriver(key string) (error, *time.Duration) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err, nil
	}
	driver, err := c.getDriver(name, namespace)
	if err != nil{
		return err, nil
	}
	// all backends are deregistered before driver is allowed to be deleted, so nothing need to be done here
	if driver.DeletionTimestamp != nil{
		return nil, nil
	}
	if len(driver.Status.Conditions) == 0{
		driver.Status = lbcfapi.LoadBalancerDriverStatus{
			Conditions: []lbcfapi.LoadBalancerDriverCondition{
				{
					Type: lbcfapi.DriverAccepted,
					Status: lbcfapi.ConditionTrue,
					LastTransitionTime: v1.Now(),
				},
			},
		}
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancerDrivers(namespace).UpdateStatus(driver)
		if err != nil{
			return err, nil
		}
	}
	return nil, nil
}

