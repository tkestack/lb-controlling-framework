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

}

func (c *Controller) deleteBackendGroup(obj interface{}) {
	c.enqueue(obj, c.backendGroupQueue)
}

func (c *Controller) addLoadBalancer(obj interface{}) {
	c.enqueue(obj, c.loadBalancerQueue)
}

func (c *Controller) updateLoadBalancer(old, cur interface{}) {

}

func (c *Controller) deleteLoadBalancer(obj interface{}) {
	c.enqueue(obj, c.loadBalancerQueue)
}

func (c *Controller) addLoadBalancerDriver(obj interface{}) {
	c.enqueue(obj, c.driverQueue)
}

func (c *Controller) updateLoadBalancerDriver(old, cur interface{}) {

}

func (c *Controller) deleteLoadBalancerDriver(obj interface{}) {
	c.enqueue(obj, c.driverQueue)
}

func (c *Controller) addBackendRecord(obj interface{}) {
}

func (c *Controller) updateBackendRecord(old, cur interface{}) {

}

func (c *Controller) deleteBackendRecord(obj interface{}) {
}
