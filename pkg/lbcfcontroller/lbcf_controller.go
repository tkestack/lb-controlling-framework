package lbcfcontroller

import (
	"time"

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app"
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
	opts *app.Options,
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
		k8sClient:         k8sClient,
		lbcfClient:        lbcfClient,
		driverQueue:       NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "driver-queue"),
		loadBalancerQueue: NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "lb-queue"),
		backendGroupQueue: NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "backendgroup-queue"),
		backendQueue: NewIntervalRateLimitingQueue(DefaultControllerRateLimiter(), "backend-queue"),
	}

	c.driverController = NewDriverController(lbcfClient, lbDriverInformer.Lister())
	c.lbController = NewLoadBalancerController(lbcfClient, lbInformer.Lister(), c.driverController)
	c.backendController = NewBackendController(lbcfClient, bgInformer.Lister(), brInformer.Lister(), c.driverController, c.lbController)

	// enqueue backendgroup
	podInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addPod,
		UpdateFunc: c.updatePod,
		DeleteFunc: c.deletePod,
	}, opts.ResyncPeriod)

	// enqueue backendgroup
	svcInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
		DeleteFunc: c.deleteService,
	}, opts.ResyncPeriod)

	// control loadBalancer lifecycle
	lbInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addLoadBalancer,
		UpdateFunc: c.updateLoadBalancer,
		DeleteFunc: c.deleteLoadBalancer,
	}, opts.ResyncPeriod)

	// test driver health
	lbDriverInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addLoadBalancerDriver,
		UpdateFunc: c.updateLoadBalancerDriver,
		DeleteFunc: c.deleteLoadBalancerDriver,
	}, opts.ResyncPeriod)

	// generate backendrecord
	bgInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBackendGroup,
		UpdateFunc: c.updateBackendGroup,
		DeleteFunc: c.deleteBackendGroup,
	}, opts.ResyncPeriod)

	// register/deregister backend
	brInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addBackendRecord,
		UpdateFunc: c.updateBackendRecord,
		DeleteFunc: c.deleteBackendRecord,
	}, opts.ResyncPeriod)

	c.podListerSynced = podInformer.Informer().HasSynced
	c.svcListerSynced = svcInformer.Informer().HasSynced
	c.LoadBalancerListerSynced = lbInformer.Informer().HasSynced
	c.DriverListerSynced = lbDriverInformer.Informer().HasSynced
	c.BackendGroupListerSynced = bgInformer.Informer().HasSynced
	c.BackendRecordListerSynced = brInformer.Informer().HasSynced

	return c
}

type Controller struct {
	k8sClient  *kubernetes.Clientset
	lbcfClient *lbcfclient.Clientset

	driverController *DriverController
	lbController     *LoadBalancerController
	backendController *BackendController

	podListerSynced           cache.InformerSynced
	svcListerSynced           cache.InformerSynced
	LoadBalancerListerSynced  cache.InformerSynced
	DriverListerSynced        cache.InformerSynced
	BackendGroupListerSynced  cache.InformerSynced
	BackendRecordListerSynced cache.InformerSynced

	driverQueue       IntervalRateLimitingInterface
	loadBalancerQueue IntervalRateLimitingInterface
	backendGroupQueue IntervalRateLimitingInterface
	backendQueue IntervalRateLimitingInterface
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
	for i := 0; i < 3; i++ {
		go wait.Until(c.lbWorker, time.Second, wait.NeverStop)
		go wait.Until(c.driverWorker, time.Second, wait.NeverStop)
		go wait.Until(c.backendGroupWorker, time.Second, wait.NeverStop)
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
	for c.processNextItem(c.loadBalancerQueue, c.lbController.syncLB) {
	}
}

func (c *Controller) driverWorker() {
	for c.processNextItem(c.driverQueue, c.driverController.syncDriver) {
	}
}

func (c *Controller) backendGroupWorker() {
	for c.processNextItem(c.backendGroupQueue, c.backendController.syncBackendGroup) {
	}
}

func (c *Controller) backendWorker() {
	for c.processNextItem(c.backendQueue, c.backendController.syncBackend) {
	}
}

type SyncFunc func(string)(error, *time.Duration)

const(
	DefaultRetryInterval = 10 * time.Second
)

func (c *Controller) processNextItem(queue IntervalRateLimitingInterface, syncFunc SyncFunc) bool {
	key, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(key)
	if err, delay := syncFunc(key.(string)); err != nil {
		if delay == nil{
			queue.AddIntervalRateLimited(key, DefaultWebhookTimeout)
		}else{
			queue.AddIntervalRateLimited(key, *delay)
		}
	} else {
		queue.Forget(key)
	}
	return true
}

