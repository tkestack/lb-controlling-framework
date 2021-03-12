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

package context

import (
	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/config"
	lbcfv1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"
	lbcfv1beta "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	lbcfclient "tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned"
	lbcfclientset "tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"tkestack.io/lb-controlling-framework/pkg/client-go/informers/externalversions"
	lbcfclientv1 "tkestack.io/lb-controlling-framework/pkg/client-go/informers/externalversions/lbcf.tkestack.io/v1"
	"tkestack.io/lb-controlling-framework/pkg/client-go/informers/externalversions/lbcf.tkestack.io/v1beta1"

	apicorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
)

func NewContext(cfg *config.Config) *Context {
	c := &Context{
		Cfg: cfg,
	}
	clientCfg := getClientConfigOrDie(cfg.KubeConfig, cfg.ClientQPS, cfg.ClientBurst)

	c.K8sClient = kubernetes.NewForConfigOrDie(clientCfg)
	c.LbcfClient = lbcfclientset.NewForConfigOrDie(clientCfg)

	c.K8sFactory = informers.NewSharedInformerFactory(c.K8sClient, cfg.InformerResyncPeriod)
	c.LbcfFactory = externalversions.NewSharedInformerFactory(c.LbcfClient, cfg.InformerResyncPeriod)

	c.PodInformer = c.K8sFactory.Core().V1().Pods()
	c.SvcInformer = c.K8sFactory.Core().V1().Services()
	c.NodeInformer = c.K8sFactory.Core().V1().Nodes()
	c.LBInformer = c.LbcfFactory.Lbcf().V1beta1().LoadBalancers()
	c.LBDriverInformer = c.LbcfFactory.Lbcf().V1beta1().LoadBalancerDrivers()
	c.BGInformer = c.LbcfFactory.Lbcf().V1beta1().BackendGroups()
	c.BRInformer = c.LbcfFactory.Lbcf().V1beta1().BackendRecords()
	c.BindInformer = c.LbcfFactory.Lbcf().V1().Binds()

	c.EventBroadCaster = record.NewBroadcaster()
	scheme := runtime.NewScheme()
	if err := lbcfv1beta.SchemeBuilder.AddToScheme(scheme); err != nil {
		klog.Fatal(err.Error())
	}
	if err := lbcfv1.SchemeBuilder.AddToScheme(scheme); err != nil {
		klog.Fatal(err.Error())
	}
	c.EventRecorder = c.EventBroadCaster.NewRecorder(scheme, apicorev1.EventSource{
		Component: "lbcf-controller",
	})
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
	NodeInformer     v1.NodeInformer
	LBInformer       v1beta1.LoadBalancerInformer
	LBDriverInformer v1beta1.LoadBalancerDriverInformer
	BGInformer       v1beta1.BackendGroupInformer
	BRInformer       v1beta1.BackendRecordInformer
	BindInformer     lbcfclientv1.BindInformer

	EventBroadCaster record.EventBroadcaster
	EventRecorder    record.EventRecorder
}

func (c *Context) Start() {
	c.K8sFactory.Start(wait.NeverStop)
	c.LbcfFactory.Start(wait.NeverStop)
	c.EventBroadCaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: c.K8sClient.CoreV1().Events("")})
}

func (c *Context) WaitForCacheSync() {
	c.K8sFactory.WaitForCacheSync(wait.NeverStop)
	c.LbcfFactory.WaitForCacheSync(wait.NeverStop)
}

func (c *Context) IsDryRun() bool {
	return c.Cfg.DryRun
}

func getClientConfigOrDie(kubeConfig string, qps float32, burst int) *rest.Config {
	var cfg *rest.Config
	if kubeConfig != "" {
		clientCfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			klog.Fatal(err)
		}
		cfg = clientCfg
	} else {
		clientCfg, err := rest.InClusterConfig()
		if err != nil {
			klog.Fatal(err)
		}
		cfg = clientCfg
	}
	cfg.QPS = qps
	cfg.Burst = burst
	return cfg
}
