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
	"reflect"
	"time"

	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// NewController creates a new LBCF-controller
func NewController(ctx *context.Context) *Controller {
	c := &Controller{
		context:           ctx,
		driverQueue:       util.NewConditionalDelayingQueue(nil, ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		loadBalancerQueue: util.NewConditionalDelayingQueue(util.QueueFilterForLB(ctx.LBInformer.Lister()), ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		backendGroupQueue: util.NewConditionalDelayingQueue(nil, ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		backendQueue:      util.NewConditionalDelayingQueue(util.QueueFilterForBackend(ctx.BRInformer.Lister()), ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		dryRun:            ctx.IsDryRun(),
	}

	c.driverCtrl = newDriverController(c.context.LbcfClient, c.context.LBDriverInformer.Lister(), c.context.IsDryRun())
	c.lbCtrl = newLoadBalancerController(
		c.context.LbcfClient,
		c.context.LBInformer.Lister(),
		ctx.LBDriverInformer.Lister(),
		ctx.EventRecorder,
		util.NewWebhookInvoker(),
		c.context.IsDryRun())
	c.backendCtrl = newBackendController(
		c.context.LbcfClient,
		c.context.BRInformer.Lister(),
		ctx.LBDriverInformer.Lister(),
		c.context.PodInformer.Lister(),
		c.context.SvcInformer.Lister(),
		c.context.NodeInformer.Lister(),
		c.context.EventRecorder,
		util.NewWebhookInvoker(),
		ctx.IsDryRun(),
	)
	c.backendGroupCtrl = newBackendGroupController(
		c.context.LbcfClient,
		c.context.LBInformer.Lister(),
		c.context.BGInformer.Lister(),
		c.context.BRInformer.Lister(),
		c.context.PodInformer.Lister(),
		c.context.SvcInformer.Lister(),
		c.context.NodeInformer.Lister(),
		ctx.IsDryRun(),
	)

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

// Controller implements LBCF-controller
type Controller struct {
	context *context.Context

	driverCtrl       *driverController
	lbCtrl           *loadBalancerController
	backendCtrl      *backendController
	backendGroupCtrl *backendGroupController

	driverQueue       util.ConditionalRateLimitingInterface
	loadBalancerQueue util.ConditionalRateLimitingInterface
	backendGroupQueue util.ConditionalRateLimitingInterface
	backendQueue      util.ConditionalRateLimitingInterface
	dryRun            bool
}

// Start starts controller in a new goroutine
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

func (c *Controller) enqueue(obj interface{}, queue util.ConditionalRateLimitingInterface) {
	if _, ok := obj.(string); ok {
		queue.Add(obj)
		return
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		klog.Errorf("enqueue failed: %v", err)
		return
	}
	queue.Add(key)
}

func (c *Controller) lbWorker() {
	for c.processNextItem("LoadBalancer", c.loadBalancerQueue, c.lbCtrl.syncLB) {
	}
}

func (c *Controller) driverWorker() {
	for c.processNextItem("LoadBalancerDriver", c.driverQueue, c.driverCtrl.syncDriver) {
	}
}

func (c *Controller) backendGroupWorker() {
	for c.processNextItem("BackendGroup", c.backendGroupQueue, c.backendGroupCtrl.syncBackendGroup) {
	}
}

func (c *Controller) backendWorker() {
	for c.processNextItem("BackendRecord", c.backendQueue, c.backendCtrl.syncBackendRecord) {
	}
}

func (c *Controller) processNextItem(kind string, queue util.ConditionalRateLimitingInterface, syncFunc func(string) *util.SyncResult) bool {
	key, quit := queue.Get()
	if quit {
		return false
	}

	go func() {
		defer queue.Done(key)

		klog.V(3).Infof("sync %s %s start", kind, key)
		startTime := time.Now()
		result := syncFunc(key.(string))
		// in dry-run mode, the result of a key is dumped, the same key will not be processed until:
		// 1. informer resynced
		// 2. objects in the cluster are updated and the informer received new events
		if c.dryRun {
			queue.Forget(key)
		} else {
			handleResult(kind, key, queue, result)
		}
		elapsed := time.Since(startTime)
		klog.V(3).Infof("sync %s %s, took %s", kind, key, elapsed.String())
	}()
	return true
}

func (c *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	for key := range c.backendGroupCtrl.listRelatedBackendGroupsForPod(pod) {
		c.enqueue(key, c.backendGroupQueue)
	}
}

func (c *Controller) updatePod(old, cur interface{}) {
	oldPod := old.(*v1.Pod)
	curPod := cur.(*v1.Pod)
	if oldPod.ResourceVersion == curPod.ResourceVersion {
		return
	}

	labelChanged := !reflect.DeepEqual(oldPod.Labels, curPod.Labels)
	statusChanged := util.PodAvailable(oldPod) != util.PodAvailable(curPod)

	if labelChanged || statusChanged {
		oldGroups := c.backendGroupCtrl.listRelatedBackendGroupsForPod(oldPod)
		groups := c.backendGroupCtrl.listRelatedBackendGroupsForPod(curPod)
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

func (c *Controller) addService(obj interface{}) {
	svc := obj.(*v1.Service)
	filter := func(group *v1beta1.BackendGroup) bool {
		return util.IsSvcMatchBackendGroup(group, svc)
	}
	keys, err := c.backendGroupCtrl.listRelatedBackendGroups(svc.Namespace, filter)
	if err != nil {
		klog.Errorf("skip svc(%s/%s) add, list backendgroup failed: %v", svc.Namespace, svc.Name, err)
	}
	for key := range keys {
		c.enqueue(key, c.backendGroupQueue)
	}
}

func (c *Controller) updateService(old, cur interface{}) {
	oldSvc := old.(*v1.Service)
	curSvc := cur.(*v1.Service)
	if oldSvc.ResourceVersion == curSvc.ResourceVersion || oldSvc.Generation == curSvc.Generation {
		return
	}
	for key := range c.backendGroupCtrl.listRelatedBackendGroupForSvc(curSvc) {
		c.enqueue(key, c.backendGroupQueue)
	}
}

func (c *Controller) deleteService(obj interface{}) {
	if _, ok := obj.(*v1.Service); ok {
		c.addService(obj)
		return
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		klog.Errorf("Couldn't get object from tombstone %#v", obj)
		return
	}
	svc, ok := tombstone.Obj.(*v1.Service)
	if !ok {
		klog.Errorf("Tombstone contained object that is not a BackendGroup: %#v", obj)
		return
	}
	c.addService(svc)
}

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

	for key := range c.backendGroupCtrl.listRelatedBackendGroupsForLB(lb) {
		c.backendGroupQueue.Add(key)
	}
}

func (c *Controller) updateLoadBalancer(old, cur interface{}) {
	oldLB := old.(*v1beta1.LoadBalancer)
	curLB := cur.(*v1beta1.LoadBalancer)
	if oldLB.ResourceVersion == curLB.ResourceVersion {
		return
	}
	if util.NeedEnqueueLB(oldLB, curLB) {
		c.enqueue(curLB, c.loadBalancerQueue)
	}
	for key := range c.backendGroupCtrl.listRelatedBackendGroupsForLB(curLB) {
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
	if util.NeedEnqueueBackend(oldObj, curObj) {
		c.enqueue(curObj, c.backendQueue)
	}
	if util.BackendRegistered(oldObj) != util.BackendRegistered(curObj) {
		if controllerRef := metav1.GetControllerOf(curObj); controllerRef != nil {
			c.enqueue(util.NamespacedNameKeyFunc(curObj.Namespace, controllerRef.Name), c.backendGroupQueue)
		}
	}
}

func (c *Controller) deleteBackendRecord(obj interface{}) {
	backend, ok := obj.(*v1beta1.BackendRecord)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		backend, ok = tombstone.Obj.(*v1beta1.BackendRecord)
		if !ok {
			klog.Errorf("Tombstone contained object that is not a BackendRecord: %#v", obj)
			return
		}
	}
	if controllerRef := metav1.GetControllerOf(backend); controllerRef != nil {
		c.enqueue(util.NamespacedNameKeyFunc(backend.Namespace, controllerRef.Name), c.backendGroupQueue)
	}
}

func handleResult(kind string, key interface{}, queue util.ConditionalRateLimitingInterface, result *util.SyncResult) {
	if result.IsFailed() {
		klog.Infof("Failed %s %s, reason: %v", kind, key, result.GetFailReason())
		queue.AddAfterMinimumDelay(key, result.GetNextRun())
	} else if result.IsRunning() {
		klog.Infof("Async %s %s", kind, key)
		queue.AddAfterMinimumDelay(key, result.GetNextRun())
	} else if result.IsPeriodic() {
		klog.Infof("Periodic %s %s", kind, key)
		queue.AddAfterFiltered(key, result.GetNextRun())
	} else {
		klog.Infof("Successfully Finished %s %s", kind, key)
		queue.Forget(key)
	}
}
