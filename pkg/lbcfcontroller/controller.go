package lbcfcontroller

import (
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/client-go/listers/core/v1"
)

type Controller struct {
	podLister           v1.PodLister
	lBLister            v1beta1.LoadBalancerLister
	lBDriverLister      v1beta1.LoadBalancerDriverLister
	backendGroupLister  v1beta1.BackendGroupLister
	backendRecordLister v1beta1.BackendRecordLister
}

func NewController() *Controller {
	return &Controller{}
}

func (c *Controller) Run() {

}
