package lbcfcontroller

import (
	"net/http"

	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions/lbcf.tke.cloud.tencent.com/v1beta1"

	"github.com/emicklei/go-restful"
	"k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type Controller struct {
	driverManager *DriverManager
}

func NewController(
	opts *app.Options,
	podInformer v1.PodInformer,
	lbInformer v1beta1.LoadBalancerInformer,
	lbDriverInformer v1beta1.LoadBalancerDriverInformer,
	bgInformer v1beta1.BackendGroupInformer,
	brInformer v1beta1.BackendRecordInformer,
) *Controller {
	podInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj interface{}, newOjb interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	}, opts.ResyncPeriod)

	lbInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj interface{}, newOjb interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	}, opts.ResyncPeriod)

	lbDriverInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj interface{}, newOjb interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	}, opts.ResyncPeriod)

	bgInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj interface{}, newOjb interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	}, opts.ResyncPeriod)

	brInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) {},
		UpdateFunc: func(oldObj interface{}, newOjb interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	}, opts.ResyncPeriod)

	return &Controller{
		driverManager: NewDriverManager(),
	}
}

func (c *Controller) Start() {
	go c.startAdmissionWebhook()
	go c.run()
}

func (c *Controller) run() {
}

func (c *Controller) startAdmissionWebhook() {
	ws := new(restful.WebService)
	ws.Path("/")

	ws.Route(ws.POST("mutateLoadBalancer").To(c.MutateAdmitLoadBalancer).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("mutateLoadBalancerDriver").To(c.MutateAdmitLoadBalancerDriver).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("mutateBackendGroup").To(c.MutateAdmitBackendGroup).
		Consumes(restful.MIME_JSON))

	ws.Route(ws.POST("validateLoadBalancer").To(c.ValidateAdmitLoadBalancer).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("validateLoadBalancerDriver").To(c.ValidateAdmitLoadBalancerDriver).
		Consumes(restful.MIME_JSON))
	ws.Route(ws.POST("validateBackendGroup").To(c.ValidateAdmitBackendGroup).
		Consumes(restful.MIME_JSON))

	restful.Add(ws)

	go func() {
		klog.Fatal(http.ListenAndServe(":443", nil))
	}()
}
