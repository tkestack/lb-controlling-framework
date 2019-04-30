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

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/config"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/controller"
)

func NewController(
	cfg *config.Config,
	k8sClient *kubernetes.Clientset,
	lbcfClient *lbcfclient.Clientset,
	podInformer v1.PodInformer,
	svcInformer v1.ServiceInformer,
	lbInformer v1beta1.LoadBalancerInformer,
	lbDriverInformer v1beta1.LoadBalancerDriverInformer,
	bgInformer v1beta1.BackendGroupInformer,
	brInformer v1beta1.BackendRecordInformer,
) *Controller {
	c := &Controller{
		cfg:               cfg,
		k8sClient:         k8sClient,
		lbcfClient:        lbcfClient,
		driverQueue:       NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "driver-queue", cfg.MinRetryDelay),
		loadBalancerQueue: NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "lb-queue", cfg.MinRetryDelay),
		backendGroupQueue: NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "backendgroup-queue", cfg.MinRetryDelay),
		backendQueue:      NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "backend-queue", cfg.MinRetryDelay),
	}

	c.driverCtrl = NewDriverController(lbcfClient, lbDriverInformer.Lister())
	c.lbCtrl = NewLoadBalancerController(lbcfClient, lbInformer.Lister(), c.driverCtrl)
	c.backendCtrl = NewBackendController(lbcfClient, brInformer.Lister(), c.driverCtrl, NewPodProvider(podInformer.Lister()))
	c.backendGroupCtrl = NewBackendGroupController(lbcfClient, lbInformer.Lister(), bgInformer.Lister(), brInformer.Lister(), NewPodProvider(podInformer.Lister()))

	// enqueue backendgroup
	podInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addPod,
		UpdateFunc: c.updatePod,
		DeleteFunc: c.deletePod,
	}, cfg.InformerResyncPeriod)

	// enqueue backendgroup
	svcInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
		DeleteFunc: c.deleteService,
	}, cfg.InformerResyncPeriod)

	// control loadBalancer lifecycle
	lbInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addLoadBalancer,
		UpdateFunc: c.updateLoadBalancer,
		DeleteFunc: c.deleteLoadBalancer,
	}, cfg.InformerResyncPeriod)

	// test driver health
	lbDriverInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addLoadBalancerDriver,
		UpdateFunc: c.updateLoadBalancerDriver,
		DeleteFunc: c.deleteLoadBalancerDriver,
	}, cfg.InformerResyncPeriod)

	// generate backendrecord
	bgInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBackendGroup,
		UpdateFunc: c.updateBackendGroup,
		DeleteFunc: c.deleteBackendGroup,
	}, cfg.InformerResyncPeriod)

	// register/deregister backend
	brInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBackendRecord,
		UpdateFunc: c.updateBackendRecord,
		DeleteFunc: c.deleteBackendRecord,
	}, cfg.InformerResyncPeriod)

	c.podListerSynced = podInformer.Informer().HasSynced
	c.svcListerSynced = svcInformer.Informer().HasSynced
	c.LoadBalancerListerSynced = lbInformer.Informer().HasSynced
	c.DriverListerSynced = lbDriverInformer.Informer().HasSynced
	c.BackendGroupListerSynced = bgInformer.Informer().HasSynced
	c.BackendRecordListerSynced = brInformer.Informer().HasSynced

	return c
}

type Controller struct {
	cfg        *config.Config
	k8sClient  *kubernetes.Clientset
	lbcfClient *lbcfclient.Clientset

	driverCtrl       *DriverController
	lbCtrl           *LoadBalancerController
	backendCtrl      *BackendController
	backendGroupCtrl *BackendGroupController

	podListerSynced           cache.InformerSynced
	svcListerSynced           cache.InformerSynced
	LoadBalancerListerSynced  cache.InformerSynced
	DriverListerSynced        cache.InformerSynced
	BackendGroupListerSynced  cache.InformerSynced
	BackendRecordListerSynced cache.InformerSynced

	driverQueue       IntervalRateLimitingInterface
	loadBalancerQueue IntervalRateLimitingInterface
	backendGroupQueue IntervalRateLimitingInterface
	backendQueue      IntervalRateLimitingInterface
}

func (c *Controller) Start() {
	go c.run()
}

func (c *Controller) run() {
	if !cache.WaitForCacheSync(wait.NeverStop,
		c.podListerSynced,
		c.svcListerSynced,
		c.LoadBalancerListerSynced,
		c.DriverListerSynced,
		c.BackendGroupListerSynced,
		c.BackendRecordListerSynced) {
		return
	}
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

type SyncFunc func(string) *SyncResult

type SyncResult struct {
	err error

	operationFailed   bool
	asyncOperation    bool
	periodicOperation bool

	minRetryDelay   *time.Duration
	minResyncPeriod *time.Duration
}

const (
	DefaultRetryInterval = 10 * time.Second
)

func (c *Controller) processNextItem(queue IntervalRateLimitingInterface, syncFunc SyncFunc) bool {
	key, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(key)

	go func() {
		result := syncFunc(key.(string))
		if result.err != nil {
			queue.AddRateLimited(key)
		} else if result.operationFailed {
			queue.AddIntervalRateLimited(key, result.minRetryDelay)
		} else if result.asyncOperation || result.periodicOperation {
			queue.Forget(key)
			queue.AddIntervalRateLimited(key, result.minRetryDelay)
		}
	}()
	return true
}
