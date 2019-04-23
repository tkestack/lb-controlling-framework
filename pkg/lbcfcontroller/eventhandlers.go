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
