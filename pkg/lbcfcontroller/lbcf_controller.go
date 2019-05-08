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
	"time"

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/util"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	for i := 0; i < 10; i++ {
		go wait.Until(c.lbWorker, time.Second, wait.NeverStop)
		go wait.Until(c.driverWorker, time.Second, wait.NeverStop)
		go wait.Until(c.backendGroupWorker, time.Second, wait.NeverStop)
		go wait.Until(c.backendWorker, time.Second, wait.NeverStop)
	}
}

func (c *Controller) enqueue(obj interface{}, queue workqueue.RateLimitingInterface) {
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
		} else if result.IsAsync() {
			klog.Infof("sync key %s, async", key)
			queue.Forget(key)
			queue.AddIntervalRateLimited(key, result.GetRetryDelay())
		} else if result.IsPeriodic() {
			klog.Infof("sync key %s, period", key)
			queue.Forget(key)
			queue.AddIntervalRateLimited(key, result.GetResyncPeriodic())
		}
	}()
	return true
}

func (c *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	groupList, err := c.backendGroupCtrl.bgLister.BackendGroups(pod.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip pod(%s/%s) add, list backendgroup failed: %v", pod.Namespace, pod.Name, err)
		return
	}
	relatedBackendGroup := util.FilterBackendGroup(groupList, func(group *v1beta1.BackendGroup) bool {
		if util.IsPodMatchBackendGroup(group, pod) {
			return true
		}
		return false
	})
	for _, g := range relatedBackendGroup {
		c.enqueue(g, c.backendGroupQueue)
	}
}

func (c *Controller) updatePod(old, cur interface{}) {
	oldPod := old.(*v1.Pod)
	curPod := cur.(*v1.Pod)
	groupList, err := c.backendGroupCtrl.bgLister.BackendGroups(curPod.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip pod(%s/%s) update, list backendgroup failed: %v", curPod.Namespace, curPod.Name, err)
		return
	}
	relatedBackendGroup := util.FilterBackendGroup(groupList, func(group *v1beta1.BackendGroup) bool {
		if util.IsPodMatchBackendGroup(group, curPod) || util.IsPodMatchBackendGroup(group, oldPod) {
			return true
		}
		return false
	})
	for _, g := range relatedBackendGroup {
		c.enqueue(g, c.backendGroupQueue)
	}
}

func (c *Controller) deletePod(obj interface{}) {
	pod := obj.(*v1.Pod)
	groupList, err := c.backendGroupCtrl.bgLister.BackendGroups(pod.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip pod(%s/%s) delete, list backendgroup failed: %v", pod.Namespace, pod.Name, err)
		return
	}
	relatedBackendGroup := util.FilterBackendGroup(groupList, func(group *v1beta1.BackendGroup) bool {
		if util.IsPodMatchBackendGroup(group, pod) {
			return true
		}
		return false
	})
	for _, g := range relatedBackendGroup {
		c.enqueue(g, c.backendGroupQueue)
	}
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
	c.enqueue(cur, c.backendGroupQueue)
}

func (c *Controller) deleteBackendGroup(obj interface{}) {
	c.enqueue(obj, c.backendGroupQueue)
}

func (c *Controller) addLoadBalancer(obj interface{}) {
	lb := obj.(*v1beta1.LoadBalancer)
	c.enqueue(obj, c.loadBalancerQueue)

	groupList, err := c.backendGroupCtrl.bgLister.BackendGroups(lb.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip loadbalancer(%s/%s) add, list backendgroup failed: %v", lb.Namespace, lb.Name, err)
	}
	relatedGroups := util.FilterBackendGroup(groupList, func(group *v1beta1.BackendGroup) bool {
		return util.IsLBMatchBackendGroup(group, lb)
	})
	for _, g := range relatedGroups {
		c.enqueue(g, c.backendGroupQueue)
	}
}

func (c *Controller) updateLoadBalancer(old, cur interface{}) {
	oldObj := cur.(*v1beta1.LoadBalancer)
	curObj := cur.(*v1beta1.LoadBalancer)
	if curObj.DeletionTimestamp != nil {
		c.enqueue(curObj, c.loadBalancerQueue)
	}
	if !util.LoadBalancerNonStatusUpdated(oldObj, curObj) {
		return
	}
	c.enqueue(curObj, c.loadBalancerQueue)
	groupList, err := c.backendGroupCtrl.bgLister.BackendGroups(curObj.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("skip loadbalancer(%s/%s) update, list backendgroup failed: %v", curObj.Namespace, curObj.Name, err)
	}
	relatedGroups := util.FilterBackendGroup(groupList, func(group *v1beta1.BackendGroup) bool {
		return util.IsLBMatchBackendGroup(group, curObj)
	})
	for _, g := range relatedGroups {
		c.enqueue(g, c.backendGroupQueue)
	}
}

func (c *Controller) deleteLoadBalancer(obj interface{}) {
	c.enqueue(obj, c.loadBalancerQueue)
	// TODO: enqueu related backendgroup
}

func (c *Controller) addLoadBalancerDriver(obj interface{}) {
	c.enqueue(obj, c.driverQueue)
}

func (c *Controller) updateLoadBalancerDriver(old, cur interface{}) {
	c.enqueue(cur, c.driverQueue)
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
	if curObj.DeletionTimestamp != nil {
		c.enqueue(curObj, c.backendQueue)
	}
	if !util.MapEqual(oldObj.Spec.Parameters, curObj.Spec.Parameters) {
		c.enqueue(curObj, c.backendQueue)
		return
	}
	if !util.EnsurePolicyEqual(oldObj.Spec.EnsurePolicy, curObj.Spec.EnsurePolicy) {
		c.enqueue(curObj, c.loadBalancerQueue)
		return
	}
}

func (c *Controller) deleteBackendRecord(obj interface{}) {
	c.enqueue(obj, c.backendQueue)
}
