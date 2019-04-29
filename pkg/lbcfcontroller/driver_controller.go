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
	obj, ok := c.driverStore.Load(namespacedNameKeyFunc(namespace, name))
	if !ok {
		return nil, false
	}
	return obj.(DriverConnector), ok
}

func (c *DriverController) syncDriver(key string) *SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return &SyncResult{err: err}
	}
	driver, err := c.lister.LoadBalancerDrivers(name).Get(namespace)
	if errors.IsNotFound(err) {
		c.driverStore.Delete(namespacedNameKeyFunc(namespace, name))
		return &SyncResult{err: err}
	} else if err != nil {
		return &SyncResult{err: err}
	}

	if driver.DeletionTimestamp != nil {
		c.driverStore.Delete(namespacedNameKeyFunc(namespace, name))
		return &SyncResult{}
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
			return &SyncResult{err: err}
		}
		c.driverStore.Store(key, NewDriverConnector(driver))
		return &SyncResult{}
	}

	// update DriverConnector
	dc, ok := c.driverStore.Load(key)
	if !ok {
		// this shouldn't happen
		return &SyncResult{}
	}
	dc.(DriverConnector).UpdateConfig(driver)
	return &SyncResult{}
}

const (
	driverDrainingLabel string = "lbcf.tke.cloud.tencent.com/driver-draining"
)

type DriverConnector interface {
	CallValidateLoadBalancer(req *ValidateLoadBalancerRequest) (*ValidateLoadBalancerResponse, error)

	CallCreateLoadBalancer(req *CreateLoadBalancerRequest) (*CreateLoadBalancerResponse, error)

	CallEnsureLoadBalancer(req *EnsureLoadBalancerRequest) (*EnsureLoadBalancerResponse, error)

	CallDeleteLoadBalancer(req *DeleteLoadBalancerRequest) (*DeleteLoadBalancerResponse, error)

	CallValidateBackend(req *ValidateBackendRequest) (*ValidateBackendResponse, error)

	CallGenerateBackendAddr(req *GenerateBackendAddrRequest) (*GenerateBackendAddrResponse, error)

	CallEnsureBackend(req *BackendOperationRequest) (*BackendOperationResponse, error)

	CallDeregisterBackend(req *BackendOperationRequest) (*BackendOperationResponse, error)

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

func (d *DriverConnectorImpl) CallValidateLoadBalancer(req *ValidateLoadBalancerRequest) (*ValidateLoadBalancerResponse, error) {
	rsp := &ValidateLoadBalancerResponse{}
	if err := callWebhook(d.GetConfig(), ValidateBEHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallCreateLoadBalancer(req *CreateLoadBalancerRequest) (*CreateLoadBalancerResponse, error) {
	rsp := &CreateLoadBalancerResponse{}
	if err := callWebhook(d.GetConfig(), CreateLBHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallEnsureLoadBalancer(req *EnsureLoadBalancerRequest) (*EnsureLoadBalancerResponse, error) {
	rsp := &EnsureLoadBalancerResponse{}
	if err := callWebhook(d.GetConfig(), EnsureLBHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallDeleteLoadBalancer(req *DeleteLoadBalancerRequest) (*DeleteLoadBalancerResponse, error) {
	rsp := &DeleteLoadBalancerResponse{}
	if err := callWebhook(d.GetConfig(), DeleteLBHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallValidateBackend(req *ValidateBackendRequest) (*ValidateBackendResponse, error) {
	rsp := &ValidateBackendResponse{}
	if err := callWebhook(d.GetConfig(), ValidateBEHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallGenerateBackendAddr(req *GenerateBackendAddrRequest) (*GenerateBackendAddrResponse, error) {
	rsp := &GenerateBackendAddrResponse{}
	if err := callWebhook(d.GetConfig(), GenerateBEAddrHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallEnsureBackend(req *BackendOperationRequest) (*BackendOperationResponse, error) {
	rsp := &BackendOperationResponse{}
	if err := callWebhook(d.GetConfig(), EnsureBEHook, req, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func (d *DriverConnectorImpl) CallDeregisterBackend(req *BackendOperationRequest) (*BackendOperationResponse, error) {
	rsp := &BackendOperationResponse{}
	if err := callWebhook(d.GetConfig(), DeregBEHook, req, rsp); err != nil {
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
	if v, ok := d.config.Labels[driverDrainingLabel]; !ok || strings.ToUpper(v) != "TRUE" {
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
