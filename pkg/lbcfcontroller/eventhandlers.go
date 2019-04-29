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

import "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"

func (c *Controller) addPod(obj interface{}) {
	// TODO: find backendGroup by pod

}

func (c *Controller) updatePod(old, cur interface{}) {

}

func (c *Controller) deletePod(obj interface{}) {

}

func (c *Controller) addService(obj interface{}) {
	// TODO: find backendGroup by service

}

func (c *Controller) updateService(old, cur interface{}) {

}

func (c *Controller) deleteService(obj interface{}) {

}

func (c *Controller) addBackendGroup(obj interface{}) {
	c.enqueue(obj, c.backendGroupQueue)
}

func (c *Controller) updateBackendGroup(old, cur interface{}) {
	// TODO: if obj changed, enqueue

}

func (c *Controller) deleteBackendGroup(obj interface{}) {
	c.enqueue(obj, c.backendGroupQueue)
}

func (c *Controller) addLoadBalancer(obj interface{}) {
	c.enqueue(obj, c.loadBalancerQueue)
}

func (c *Controller) updateLoadBalancer(old, cur interface{}) {
	oldObj := cur.(*v1beta1.LoadBalancer)
	curObj := cur.(*v1beta1.LoadBalancer)
	if !equalMap(oldObj.Spec.Attributes, curObj.Spec.Attributes) {
		c.enqueue(curObj, c.loadBalancerQueue)
		return
	}
	if !equalResyncPolicy(oldObj.Spec.ResyncPolicy, curObj.Spec.ResyncPolicy) {
		c.enqueue(curObj, c.loadBalancerQueue)
		return
	}
}

func (c *Controller) deleteLoadBalancer(obj interface{}) {
	c.enqueue(obj, c.loadBalancerQueue)
}

func (c *Controller) addLoadBalancerDriver(obj interface{}) {
	c.enqueue(obj, c.driverQueue)
}

func (c *Controller) updateLoadBalancerDriver(old, cur interface{}) {
	// TODO: if obj changed, enqueue

}

func (c *Controller) deleteLoadBalancerDriver(obj interface{}) {
	c.enqueue(obj, c.driverQueue)
}

func (c *Controller) addBackendRecord(obj interface{}) {
	c.enqueue(obj, c.backendQueue)
}

func (c *Controller) updateBackendRecord(old, cur interface{}) {
	oldObj := cur.(*v1beta1.BackendRecord)
	curObj := cur.(*v1beta1.BackendRecord)
	if !equalMap(oldObj.Spec.Parameters, curObj.Spec.Parameters) {
		c.enqueue(curObj, c.backendQueue)
		return
	}
	if !equalResyncPolicy(oldObj.Spec.ResyncPolicy, curObj.Spec.ResyncPolicy) {
		c.enqueue(curObj, c.loadBalancerQueue)
		return
	}
}

func (c *Controller) deleteBackendRecord(obj interface{}) {
	c.enqueue(obj, c.backendQueue)
}
