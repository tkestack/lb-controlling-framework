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
	"strings"
	"sync"

	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type DriverProvider interface {
	getDriver(namespace, name string) (DriverConnector, bool)
}

func NewDriverController(client *lbcfclient.Clientset, lister v1beta1.LoadBalancerDriverLister) *DriverController {
	return &DriverController{
		lbcfClient:  client,
		lister:      lister,
		driverStore: &sync.Map{},
	}
}

type DriverController struct {
	lbcfClient *lbcfclient.Clientset
	lister     v1beta1.LoadBalancerDriverLister

	// driver name --> DriverConnector
	driverStore *sync.Map
}

func (c *DriverController) getDriver(namespace, name string) (DriverConnector, bool) {
	obj, ok := c.driverStore.Load(util.NamespacedNameKeyFunc(namespace, name))
	if !ok {
		return nil, false
	}
	return obj.(DriverConnector), ok
}

func (c *DriverController) syncDriver(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	driver, err := c.lister.LoadBalancerDrivers(name).Get(namespace)
	if errors.IsNotFound(err) {
		c.driverStore.Delete(util.NamespacedNameKeyFunc(namespace, name))
		return util.ErrorResult(err)
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if driver.DeletionTimestamp != nil {
		c.driverStore.Delete(util.NamespacedNameKeyFunc(namespace, name))
		return util.SuccResult()
	}

	// create DriverConnector
	if len(driver.Status.Conditions) == 0 {
		driver.Status = lbcfapi.LoadBalancerDriverStatus{
			Conditions: []lbcfapi.LoadBalancerDriverCondition{
				{
					Type:               lbcfapi.DriverAccepted,
					Status:             lbcfapi.ConditionTrue,
					LastTransitionTime: v1.Now(),
				},
			},
		}
		_, err := c.lbcfClient.LbcfV1beta1().LoadBalancerDrivers(namespace).UpdateStatus(driver)
		if err != nil {
			return util.ErrorResult(err)
		}
		c.driverStore.Store(key, NewDriverConnector(driver))
		return util.SuccResult()
	}

	// update DriverConnector
	dc, ok := c.driverStore.Load(key)
	if !ok {
		// this shouldn't happen
		return util.SuccResult()
	}
	dc.(DriverConnector).UpdateConfig(driver)
	return util.SuccResult()
}

type DriverConnector interface {
	CallValidateLoadBalancer(req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error)

	CallCreateLoadBalancer(req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error)

	CallEnsureLoadBalancer(req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error)

	CallDeleteLoadBalancer(req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error)

	CallValidateBackend(req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error)

	CallGenerateBackendAddr(req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error)

	CallEnsureBackend(req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error)

	CallDeregisterBackend(req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error)

	GetConfig() *lbcfapi.LoadBalancerDriver

	UpdateConfig(*lbcfapi.LoadBalancerDriver)

	IsDraining() bool

	Start()
}

func NewDriverConnector(config *lbcfapi.LoadBalancerDriver) DriverConnector {
	return &DriverConnectorImpl{
		config: config.DeepCopy(),
	}
}

type DriverConnectorImpl struct {
	sync.RWMutex

	config *lbcfapi.LoadBalancerDriver
}

func (d *DriverConnectorImpl) CallValidateLoadBalancer(req *webhooks.ValidateLoadBalancerRequest) (*webhooks.ValidateLoadBalancerResponse, error) {
	rsp := &webhooks.ValidateLoadBalancerResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.ValidateBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallCreateLoadBalancer(req *webhooks.CreateLoadBalancerRequest) (*webhooks.CreateLoadBalancerResponse, error) {
	rsp := &webhooks.CreateLoadBalancerResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.CreateLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallEnsureLoadBalancer(req *webhooks.EnsureLoadBalancerRequest) (*webhooks.EnsureLoadBalancerResponse, error) {
	rsp := &webhooks.EnsureLoadBalancerResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.EnsureLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallDeleteLoadBalancer(req *webhooks.DeleteLoadBalancerRequest) (*webhooks.DeleteLoadBalancerResponse, error) {
	rsp := &webhooks.DeleteLoadBalancerResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.DeleteLoadBalancer, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallValidateBackend(req *webhooks.ValidateBackendRequest) (*webhooks.ValidateBackendResponse, error) {
	rsp := &webhooks.ValidateBackendResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.ValidateBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallGenerateBackendAddr(req *webhooks.GenerateBackendAddrRequest) (*webhooks.GenerateBackendAddrResponse, error) {
	rsp := &webhooks.GenerateBackendAddrResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.GenerateBackendAddr, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallEnsureBackend(req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	rsp := &webhooks.BackendOperationResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.EnsureBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallDeregisterBackend(req *webhooks.BackendOperationRequest) (*webhooks.BackendOperationResponse, error) {
	rsp := &webhooks.BackendOperationResponse{}
	if err := util.CallWebhook(d.GetConfig(), webhooks.DeregBackend, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) UpdateConfig(obj *lbcfapi.LoadBalancerDriver) {
	d.Lock()
	defer d.Unlock()
	d.config = obj.DeepCopy()
}

func (d *DriverConnectorImpl) IsDraining() bool {
	d.RLock()
	defer d.RUnlock()
	if v, ok := d.config.Labels[lbcfapi.DriverDrainingLabel]; !ok || strings.ToUpper(v) != "TRUE" {
		return true
	}
	return false
}

func (d *DriverConnectorImpl) Start() {
	panic("implement me")
}

func (d *DriverConnectorImpl) GetConfig() *lbcfapi.LoadBalancerDriver {
	d.RLock()
	defer d.Unlock()
	return d.config.DeepCopy()
}
