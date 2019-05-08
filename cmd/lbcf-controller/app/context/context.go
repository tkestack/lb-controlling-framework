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

package context

import (
	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/config"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	lbcfclientset "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions/lbcf.tke.cloud.tencent.com/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

func NewContext(cfg *config.Config) *Context {
	c := &Context{
		Cfg: cfg,
	}
	clientCfg := getClientConfigOrDie(cfg.KubeConfig)

	c.K8sClient = kubernetes.NewForConfigOrDie(clientCfg)
	c.LbcfClient = lbcfclientset.NewForConfigOrDie(clientCfg)

	c.K8sFactory = informers.NewSharedInformerFactory(c.K8sClient, cfg.InformerResyncPeriod)
	c.LbcfFactory = externalversions.NewSharedInformerFactory(c.LbcfClient, cfg.InformerResyncPeriod)

	c.PodInformer = c.K8sFactory.Core().V1().Pods()
	c.SvcInformer = c.K8sFactory.Core().V1().Services()
	c.LBInformer = c.LbcfFactory.Lbcf().V1beta1().LoadBalancers()
	c.LBDriverInformer = c.LbcfFactory.Lbcf().V1beta1().LoadBalancerDrivers()
	c.BGInformer = c.LbcfFactory.Lbcf().V1beta1().BackendGroups()
	c.BRInformer = c.LbcfFactory.Lbcf().V1beta1().BackendRecords()
	return c
}

type Context struct {
	Cfg *config.Config

	K8sClient  *kubernetes.Clientset
	LbcfClient *lbcfclient.Clientset

	K8sFactory  informers.SharedInformerFactory
	LbcfFactory externalversions.SharedInformerFactory

	PodInformer      v1.PodInformer
	SvcInformer      v1.ServiceInformer
	LBInformer       v1beta1.LoadBalancerInformer
	LBDriverInformer v1beta1.LoadBalancerDriverInformer
	BGInformer       v1beta1.BackendGroupInformer
	BRInformer       v1beta1.BackendRecordInformer
}

func (c *Context) WaitForCacheSync() {
	c.K8sFactory.WaitForCacheSync(wait.NeverStop)
	c.LbcfFactory.WaitForCacheSync(wait.NeverStop)
}

func getClientConfigOrDie(kubeConfig string) *rest.Config {
	if kubeConfig != "" {
		clientCfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			klog.Fatal(err)
		}
		return clientCfg
	}
	clientCfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatal(err)
	}
	return clientCfg
}
