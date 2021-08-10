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
	"fmt"
	"reflect"
	"sync"
	"time"

	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/bindcontroller"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/context"
	bindutil "tkestack.io/lb-controlling-framework/pkg/api/bind"
	"tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/metrics"
)

// NewController creates a new LBCF-controller
func NewController(ctx *context.Context) *Controller {
	c := &Controller{
		context:           ctx,
		driverQueue:       util.NewConditionalDelayingQueue("LoadBalancerDriver", nil, ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		loadBalancerQueue: util.NewConditionalDelayingQueue("LoadBalancer", util.QueueFilterForLB(ctx.LBInformer.Lister()), ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		backendGroupQueue: util.NewConditionalDelayingQueue("BackendGroup", nil, ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		backendQueue:      util.NewConditionalDelayingQueue("BackendRecord", util.QueueFilterForBackend(ctx.BRInformer.Lister()), ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
		bindQueue:         util.NewConditionalDelayingQueue("Bind", util.QueueFilterForBackend(ctx.BRInformer.Lister()), ctx.Cfg.MinRetryDelay, ctx.Cfg.RetryDelayStep, ctx.Cfg.MaxRetryDelay),
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
		c.context.LBDriverInformer.Lister(),
		c.context.LBInformer.Lister(),
		c.context.BGInformer.Lister(),
		c.context.BRInformer.Lister(),
		c.context.PodInformer.Lister(),
		c.context.SvcInformer.Lister(),
		c.context.NodeInformer.Lister(),
		util.NewWebhookInvoker(),
		ctx.EventRecorder,
		ctx.IsDryRun(),
	)
	c.bindController = bindcontroller.NewController(
		c.context.LbcfClient,
		c.context.LBDriverInformer.Lister(),
		c.context.BindInformer.Lister(),
		c.context.BRInformer.Lister(),
		c.context.PodInformer.Lister(),
		util.NewWebhookInvoker(),
		ctx.EventRecorder,
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

	c.context.BindInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBind,
		UpdateFunc: c.updateBind,
		DeleteFunc: c.deleteBind,
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
	bindController   *bindcontroller.Controller

	driverQueue       util.ConditionalRateLimitingInterface
	loadBalancerQueue util.ConditionalRateLimitingInterface
	backendGroupQueue util.ConditionalRateLimitingInterface
	backendQueue      util.ConditionalRateLimitingInterface
	bindQueue         util.ConditionalRateLimitingInterface
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
	go wait.Until(c.updateQueuePendingMetric, 10*time.Second, wait.NeverStop)
	go wait.Until(c.bindWorker, time.Second, wait.NeverStop)
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

func (c *Controller) bindWorker() {
	for c.processNextItem(c.bindQueue, c.bindController.Sync) {
	}
}

func (c *Controller) processNextItem(queue util.ConditionalRateLimitingInterface, syncFunc func(string) *util.SyncResult) bool {
	key, quit := queue.Get()
	if quit {
		return false
	}

	go func() {
		metrics.WorkingKeysInc(queue.GetName())
		// in dry-run mode, each key is processed only once
		if !c.dryRun {
			defer queue.Done(key)
		}

		klog.V(3).Infof("sync %s %s start", queue.GetName(), key)
		startTime := time.Now()
		result := syncFunc(key.(string))

		// reset rate limiter if not failed
		if !result.IsFailed() {
			queue.Forget(key)
		}
		// handle result
		if result.IsFailed() {
			klog.Infof("Failed %s %s, reason: %v", queue.GetName(), key, result.GetFailReason())
			queue.AddAfterMinimumDelay(key, result.GetNextRun())
		} else if result.IsRunning() {
			klog.Infof("Async %s %s", queue.GetName(), key)
			queue.AddAfterMinimumDelay(key, result.GetNextRun())
		} else if result.IsPeriodic() {
			klog.Infof("Periodic %s %s", queue.GetName(), key)
			queue.AddAfterFiltered(key, result.GetNextRun())
		} else {
			klog.Infof("Successfully Finished %s %s", queue.GetName(), key)
		}

		elapsed := time.Since(startTime)
		klog.V(3).Infof("sync %s %s, took %s", queue.GetName(), key, elapsed.String())
		metrics.KeyProcessLatencyObserve(queue.GetName(), elapsed)
		metrics.WorkingKeysDec(queue.GetName())
	}()
	return true
}

func (c *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	klog.V(4).Infof("receive pod %s/%s create event", pod.Namespace, pod.Name)
	// maybe we can ignore Added Pod? For
	// 1. Added Pods are never ready
	// 2. every time we restart, all LBCF CRDs are synced
	for key := range c.backendGroupCtrl.listRelatedBackendGroupsForPod(pod) {
		c.enqueue(key, c.backendGroupQueue)
	}
	for key := range c.bindController.ListRelatedBindForPod(pod) {
		c.enqueue(key, c.bindQueue)
	}
}

func (c *Controller) updatePod(old, cur interface{}) {
	oldPod := old.(*v1.Pod)
	curPod := cur.(*v1.Pod)
	klog.V(4).Infof("receive pod %s/%s update event", curPod.Namespace, curPod.Name)
	if oldPod.ResourceVersion == curPod.ResourceVersion {
		return
	}

	labelChanged := !reflect.DeepEqual(oldPod.Labels, curPod.Labels)
	statusChanged := util.PodAvailable(oldPod) != util.PodAvailable(curPod)

	klog.Infof("old pod %s/%s new pod %s/%s statusChanged %t", oldPod.Namespace, oldPod.Name, curPod.Namespace, curPod.Name, statusChanged)

	// TODO:DeregisterWebhook
	if statusChanged && (!util.PodAvailable(curPod) || !util.PodAvailableByRunning(curPod)) {
		err := c.handlePodStatusChanged(curPod)
		if err != nil {
			klog.Errorf("failed to handle pod %s/%s status change: %v", curPod.Namespace, curPod.Name, err)
		}
	}

	if labelChanged || statusChanged {
		oldGroups := c.backendGroupCtrl.listRelatedBackendGroupsForPod(oldPod)
		groups := c.backendGroupCtrl.listRelatedBackendGroupsForPod(curPod)
		groups = util.UnionOrDifferenceUnion(oldGroups, groups, statusChanged)
		for key := range groups {
			c.enqueue(key, c.backendGroupQueue)
		}

		oldBinds := c.bindController.ListRelatedBindForPod(oldPod)
		curBinds := c.bindController.ListRelatedBindForPod(curPod)
		curBinds = util.UnionOrDifferenceUnion(oldBinds, curBinds, statusChanged)
		for key := range curBinds {
			c.enqueue(key, c.bindQueue)
		}
	}
}

func (c *Controller) deletePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			klog.Errorf("Tombstone contained object that is not a Pod: %#v", obj)
			return
		}
	}
	klog.V(4).Infof("receive pod %s/%s delete event", pod.Namespace, pod.Name)
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
		c.enqueue(key, c.backendGroupQueue)
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
	backend := obj.(*v1beta1.BackendRecord)
	klog.V(4).Infof("receive backendrecord %s/%s create event", backend.Namespace, backend.Name)
	alwaysEnsure := backend.Spec.EnsurePolicy != nil && backend.Spec.EnsurePolicy.Policy == v1beta1.PolicyAlways
	if !util.BackendRegistered(backend) || alwaysEnsure || backend.DeletionTimestamp != nil {
		c.enqueue(obj, c.backendQueue)
	}
}

func (c *Controller) updateBackendRecord(old, cur interface{}) {
	oldObj := old.(*v1beta1.BackendRecord)
	curObj := cur.(*v1beta1.BackendRecord)
	klog.V(4).Infof("receive backendrecord %s/%s update event", curObj.Namespace, curObj.Name)
	if oldObj.ResourceVersion == curObj.ResourceVersion {
		return
	}
	if util.NeedEnqueueBackend(oldObj, curObj) {
		c.enqueue(curObj, c.backendQueue)
	}
	if util.BackendRegistered(oldObj) != util.BackendRegistered(curObj) {
		if controllerRef := metav1.GetControllerOf(curObj); controllerRef != nil {
			switch controllerRef.Kind {
			case "Bind":
				c.enqueue(util.NamespacedNameKeyFunc(curObj.Namespace, controllerRef.Name), c.bindQueue)
			case "BackendGroup":
				c.enqueue(util.NamespacedNameKeyFunc(curObj.Namespace, controllerRef.Name), c.backendGroupQueue)
			}
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
	klog.V(4).Infof("receive backendrecord %s/%s delete event", backend.Namespace, backend.Name)
	if controllerRef := metav1.GetControllerOf(backend); controllerRef != nil {
		switch controllerRef.Kind {
		case "Bind":
			c.enqueue(util.NamespacedNameKeyFunc(backend.Namespace, controllerRef.Name), c.bindQueue)
		case "BackendGroup":
			c.enqueue(util.NamespacedNameKeyFunc(backend.Namespace, controllerRef.Name), c.backendGroupQueue)
		}
	}
}

func (c *Controller) addBind(obj interface{}) {
	c.enqueue(obj, c.bindQueue)
}

func (c *Controller) updateBind(old, cur interface{}) {
	c.enqueue(cur, c.bindQueue)
}

func (c *Controller) deleteBind(obj interface{}) {
	c.addBind(obj)
}

func (c *Controller) updateQueuePendingMetric() {
	metrics.PendingKeysSet(c.driverQueue.GetName(), float64(c.driverQueue.Len()))
	metrics.PendingKeysSet(c.loadBalancerQueue.GetName(), float64(c.loadBalancerQueue.Len()))
	metrics.PendingKeysSet(c.backendGroupQueue.GetName(), float64(c.backendGroupQueue.Len()))
	metrics.PendingKeysSet(c.backendQueue.GetName(), float64(c.backendQueue.Len()))
	metrics.PendingKeysSet(c.bindQueue.GetName(), float64(c.bindQueue.Len()))
}

// handlePodStatusChanged delete pod's BackendRecord directly when pod needs to be deregistered
// please note that BackendRecord won't be removed instantly for all BackendRecords are created with finalizers
func (c *Controller) handlePodStatusChanged(pod *v1.Pod) error {
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	needDelete := make([]*v1beta1.BackendRecord, 0)
	brs := c.backendCtrl.listRelatedBackendRecordsForPod(pod)
	klog.V(3).Infof("related BackendRecords for status changed pod %s/%s: %v", pod.Namespace, pod.Name, brs)
	for brKey := range brs {
		key := brKey
		wg.Add(1)
		go func() {
			namespace, name, err := cache.SplitMetaNamespaceKey(key)
			if err != nil {
				klog.Errorf("failed to handle pod status changed for BackendRecord %s: %v, skipping", key, err)
				return
			}
			br, err := c.backendCtrl.brLister.BackendRecords(namespace).Get(name)
			if err != nil {
				klog.Errorf("failed to get BackendRecord %s/%s: %v, skipping", namespace, name, err)
				return
			}
			controllerRef := metav1.GetControllerOf(br)
			if controllerRef == nil {
				klog.Warningf("BackendRecord %s/%s does not contain controllerRef, skipping", namespace, name)
				return
			}
			switch controllerRef.Kind {
			case "Bind":
				bind, err := c.context.BindInformer.Lister().Binds(namespace).Get(controllerRef.Name)
				if err != nil {
					klog.Errorf("failed to get Bind %s/%s: %v, skipping", namespace, controllerRef.Name, err)
					return
				}
				if bindutil.DeregIfNotRunning(bind) && util.PodAvailableByRunning(pod) {
					return
				}
				if bindutil.DeregByWebhook(bind) {
					// todo
					return
				}
				if util.PodAvailable(pod) {
					return
				}
			case "BackendGroup":
				bg, err := c.backendGroupCtrl.bgLister.BackendGroups(namespace).Get(controllerRef.Name)
				if err != nil {
					klog.Errorf("failed to get BackendGroup %s/%s: %v, skipping", namespace, controllerRef.Name, err)
					return
				}
				if util.DeregIfNotRunning(bg) && util.PodAvailableByRunning(pod) {
					return
				}
				if util.DeregByWebhook(bg) {
					// todo
					return
				}
				if util.PodAvailable(pod) {
					return
				}
			}
			lock.Lock()
			defer lock.Unlock()
			needDelete = append(needDelete, br)
			wg.Done()
		}()
	}
	wg.Wait()
	if klog.V(3) {
		needDeleteInfo := make([]string, 0)
		for _, br := range needDelete {
			needDeleteInfo = append(needDeleteInfo, fmt.Sprintf("%s/%s", br.Namespace, br.Name))
		}
		klog.V(3).Infof("handlePodStatusChanged %s/%s needDeleteInfo: %v", pod.Namespace, pod.Name, needDeleteInfo)
	}
	if err := util.IterateBackends(needDelete, c.backendGroupCtrl.deleteBackendRecord); err != nil {
		return err
	}
	return nil
}
