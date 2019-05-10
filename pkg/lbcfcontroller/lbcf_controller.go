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
	"reflect"
	"time"

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
)

func NewController(
	context *context.Context) *Controller {
	c := &Controller{
		context:           context,
		driverQueue:       util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "driver-queue", context.Cfg.MinRetryDelay),
		loadBalancerQueue: util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "lb-queue", context.Cfg.MinRetryDelay),
		backendGroupQueue: util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "backendgroup-queue", context.Cfg.MinRetryDelay),
		backendQueue:      util.NewIntervalRateLimitingQueue(util.DefaultControllerRateLimiter(), "backend-queue", context.Cfg.MinRetryDelay),
	}

	c.driverCtrl = NewDriverController(c.context.LbcfClient, c.context.LBDriverInformer.Lister())
	c.lbCtrl = NewLoadBalancerController(c.context.LbcfClient, c.context.LBInformer.Lister(), context.LBDriverInformer.Lister(), util.NewWebhookInvoker())
	c.backendCtrl = NewBackendController(c.context.LbcfClient, c.context.BRInformer.Lister(), context.LBDriverInformer.Lister(), c.context.PodInformer.Lister(), util.NewWebhookInvoker())
	c.backendGroupCtrl = NewBackendGroupController(c.context.LbcfClient, c.context.LBInformer.Lister(), c.context.BGInformer.Lister(), c.context.BRInformer.Lister(), c.context.PodInformer.Lister())

	// enqueue backendgroup
	c.context.PodInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addPod,
		UpdateFunc: c.updatePod,
		DeleteFunc: c.deletePod,
	}, c.context.Cfg.InformerResyncPeriod)

	// enqueue backendgroup
	c.context.SvcInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
		DeleteFunc: c.deleteService,
	}, c.context.Cfg.InformerResyncPeriod)

	// control loadBalancer lifecycle
	c.context.LBInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addLoadBalancer,
		UpdateFunc: c.updateLoadBalancer,
		DeleteFunc: c.deleteLoadBalancer,
	}, c.context.Cfg.InformerResyncPeriod)

	// test driver health
	c.context.LBDriverInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addLoadBalancerDriver,
		UpdateFunc: c.updateLoadBalancerDriver,
		DeleteFunc: c.deleteLoadBalancerDriver,
	}, c.context.Cfg.InformerResyncPeriod)

	// generate backendrecord
	c.context.BGInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBackendGroup,
		UpdateFunc: c.updateBackendGroup,
		DeleteFunc: c.deleteBackendGroup,
	}, c.context.Cfg.InformerResyncPeriod)

	// register/deregister backend
	c.context.BRInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBackendRecord,
		UpdateFunc: c.updateBackendRecord,
		DeleteFunc: c.deleteBackendRecord,
	}, c.context.Cfg.InformerResyncPeriod)
	return c
}

type Controller struct {
	context *context.Context

	driverCtrl       *DriverController
	lbCtrl           *LoadBalancerController
	backendCtrl      *BackendController
	backendGroupCtrl *BackendGroupController

	driverQueue       util.IntervalRateLimitingInterface
	loadBalancerQueue util.IntervalRateLimitingInterface
	backendGroupQueue util.IntervalRateLimitingInterface
	backendQueue      util.IntervalRateLimitingInterface
}

func (c *Controller) Start() {
	go c.run()
}

func (c *Controller) run() {
	c.context.WaitForCacheSync()
	go wait.Until(c.lbWorker, time.Second, wait.NeverStop)
	go wait.Until(c.driverWorker, time.Second, wait.NeverStop)
	go wait.Until(c.backendGroupWorker, time.Second, wait.NeverStop)
	go wait.Until(c.backendWorker, time.Second, wait.NeverStop)
}

func (c *Controller) enqueue(obj interface{}, queue workqueue.RateLimitingInterface) {
	if _, ok := obj.(string); ok {
		queue.Add(obj)
		return
	}
	key, err := controller.KeyFunc(obj)
	if err != nil {
		klog.Errorf("enqueue failed: %v", err)
		return
	}
	queue.Add(key)
}

func (c *Controller) lbWorker() {
	for c.processNextItem(c.loadBalancerQueue, c.lbCtrl.syncLB) {
	}
}

func (c *Controller) driverWorker() {
	for c.processNextItem(c.driverQueue, c.driverCtrl.syncDriver) {
	}
}

func (c *Controller) backendGroupWorker() {
	for c.processNextItem(c.backendGroupQueue, c.backendGroupCtrl.syncBackendGroup) {
	}
}

func (c *Controller) backendWorker() {
	for c.processNextItem(c.backendQueue, c.backendCtrl.syncBackendRecord) {
	}
}

func (c *Controller) processNextItem(queue util.IntervalRateLimitingInterface, syncFunc util.SyncFunc) bool {
	key, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(key)

	go func() {
		result := syncFunc(key.(string))
		if result.IsError() {
			klog.Errorf("sync key %s, err: %v", key, result.GetError())
			queue.AddRateLimited(key)
		} else if result.IsFailed() {
			klog.Infof("sync key %s, failed", key)
			queue.AddIntervalRateLimited(key, result.GetRetryDelay())
		} else if result.IsRunning() {
			klog.Infof("sync key %s, async", key)
			queue.Forget(key)
			queue.AddIntervalRateLimited(key, result.GetRetryDelay())
		} else if result.IsPeriodic() {
			klog.Infof("sync key %s, period", key)
			queue.Forget(key)
			queue.AddIntervalRateLimited(key, result.GetReEnsurePeriodic())
		}
	}()
	return true
}

func (c *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	for key := range c.backendGroupCtrl.getBackendGroupsForPod(pod) {
		c.enqueue(key, c.backendGroupQueue)
	}
}

func (c *Controller) updatePod(old, cur interface{}) {
	oldPod := old.(*v1.Pod)
	curPod := cur.(*v1.Pod)

	labelChanged := !reflect.DeepEqual(oldPod.Labels, curPod.Labels)
	statusChanged := util.PodAvailable(oldPod) != util.PodAvailable(curPod)

	if labelChanged || statusChanged {
		oldGroups := c.backendGroupCtrl.getBackendGroupsForPod(oldPod)
		groups := c.backendGroupCtrl.getBackendGroupsForPod(curPod)
		groups = util.DetermineNeededBackendGroupUpdates(oldGroups, groups, statusChanged)
		for key := range groups {
			c.enqueue(key, c.backendGroupQueue)
		}
	}
}

func (c *Controller) deletePod(obj interface{}) {
	if _, ok := obj.(*v1.Pod); ok {
		c.addPod(obj)
		return
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		klog.Errorf("Couldn't get object from tombstone %#v", obj)
		return
	}
	pod, ok := tombstone.Obj.(*v1.Pod)
	if !ok {
		klog.Errorf("Tombstone contained object that is not a Pod: %#v", obj)
		return
	}
	c.addPod(pod)
}

// TODO: implement this
func (c *Controller) addService(obj interface{}) {}

// TODO: implement this
func (c *Controller) updateService(old, cur interface{}) {}

// TODO: implement this
func (c *Controller) deleteService(obj interface{}) {}

func (c *Controller) addBackendGroup(obj interface{}) {
	c.enqueue(obj, c.backendGroupQueue)
}

func (c *Controller) updateBackendGroup(old, cur interface{}) {
	oldGroup := old.(*v1beta1.BackendGroup)
	curGroup := cur.(*v1beta1.BackendGroup)
	if oldGroup.ResourceVersion == curGroup.ResourceVersion {
		return
	}
	c.enqueue(cur, c.backendGroupQueue)
}

func (c *Controller) deleteBackendGroup(obj interface{}) {
	if _, ok := obj.(*v1beta1.BackendGroup); ok {
		c.addBackendGroup(obj)
		return
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		klog.Errorf("Couldn't get object from tombstone %#v", obj)
		return
	}
	group, ok := tombstone.Obj.(*v1beta1.BackendGroup)
	if !ok {
		klog.Errorf("Tombstone contained object that is not a BackendGroup: %#v", obj)
		return
	}
	c.addBackendGroup(group)
}

func (c *Controller) addLoadBalancer(obj interface{}) {
	lb := obj.(*v1beta1.LoadBalancer)
	c.enqueue(obj, c.loadBalancerQueue)
	for key := range c.backendGroupCtrl.getBackendGroupsForLoadBalancer(lb) {
		c.backendGroupQueue.Add(key)
	}
}

func (c *Controller) updateLoadBalancer(old, cur interface{}) {
	oldObj := old.(*v1beta1.LoadBalancer)
	curObj := cur.(*v1beta1.LoadBalancer)

	if oldObj.ResourceVersion == curObj.ResourceVersion {
		return
	}
	c.enqueue(curObj, c.loadBalancerQueue)
	for key := range c.backendGroupCtrl.getBackendGroupsForLoadBalancer(curObj) {
		c.enqueue(key, c.backendGroupQueue)
	}
}

func (c *Controller) deleteLoadBalancer(obj interface{}) {
	if _, ok := obj.(*v1beta1.LoadBalancer); ok {
		c.addLoadBalancer(obj)
		return
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		klog.Errorf("Couldn't get object from tombstone %#v", obj)
		return
	}
	lb, ok := tombstone.Obj.(*v1beta1.LoadBalancer)
	if !ok {
		klog.Errorf("Tombstone contained object that is not a LoadBalancer: %#v", obj)
		return
	}
	c.addLoadBalancer(lb)
}

func (c *Controller) addLoadBalancerDriver(obj interface{}) {
	c.enqueue(obj, c.driverQueue)
}

func (c *Controller) updateLoadBalancerDriver(old, cur interface{}) {
	oldDriver := old.(*v1beta1.LoadBalancerDriver)
	curDriver := cur.(*v1beta1.LoadBalancerDriver)
	if oldDriver.ResourceVersion == curDriver.ResourceVersion {
		return
	}
	c.enqueue(cur, c.driverQueue)
}

func (c *Controller) deleteLoadBalancerDriver(obj interface{}) {
	if _, ok := obj.(*v1beta1.LoadBalancerDriver); ok {
		c.addLoadBalancerDriver(obj)
		return
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		klog.Errorf("Couldn't get object from tombstone %#v", obj)
		return
	}
	driver, ok := tombstone.Obj.(*v1beta1.LoadBalancerDriver)
	if !ok {
		klog.Errorf("Tombstone contained object that is not a LoadBalancerDriver: %#v", obj)
		return
	}
	c.addLoadBalancerDriver(driver)
}

func (c *Controller) addBackendRecord(obj interface{}) {
	c.enqueue(obj, c.backendQueue)
}

func (c *Controller) updateBackendRecord(old, cur interface{}) {
	oldObj := old.(*v1beta1.BackendRecord)
	curObj := cur.(*v1beta1.BackendRecord)

	if oldObj.ResourceVersion == curObj.ResourceVersion {
		return
	}
	c.enqueue(curObj, c.backendQueue)
}

func (c *Controller) deleteBackendRecord(obj interface{}) {
	if _, ok := obj.(*v1beta1.BackendRecord); ok {
		c.addBackendRecord(obj)
		return
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		klog.Errorf("Couldn't get object from tombstone %#v", obj)
		return
	}
	backend, ok := tombstone.Obj.(*v1beta1.BackendRecord)
	if !ok {
		klog.Errorf("Tombstone contained object that is not a BackendRecord: %#v", obj)
		return
	}
	c.addBackendRecord(backend)
}
