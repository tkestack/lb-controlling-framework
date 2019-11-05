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

package lbcfcontroller

import (
	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func newDriverController(client lbcfclient.Interface, lister v1beta1.LoadBalancerDriverLister) *driverController {
	return &driverController{
		lbcfClient: client,
		lister:     lister,
	}
}

type driverController struct {
	lbcfClient lbcfclient.Interface
	lister     v1beta1.LoadBalancerDriverLister
}

func (c *driverController) syncDriver(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	driver, err := c.lister.LoadBalancerDrivers(namespace).Get(name)
	if errors.IsNotFound(err) {
		return util.FinishedResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}

	if driver.DeletionTimestamp != nil {
		return util.FinishedResult()
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
		return util.FinishedResult()
	}
	return util.FinishedResult()
}
