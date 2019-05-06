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

package app

import (
	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/config"
	lbcfclientset "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

func NewServer() *cobra.Command {
	cfg := config.NewConfig()

	cmd := &cobra.Command{
		Use: "lbcf-controller",

		Run: func(cmd *cobra.Command, args []string) {
			klog.Infof("hello world")
			clientCfg := getClientConfigOrDie(cfg.KubeConfig)
			k8sClient := kubernetes.NewForConfigOrDie(clientCfg)
			lbcfClient := lbcfclientset.NewForConfigOrDie(clientCfg)
			k8sFactory := informers.NewSharedInformerFactory(k8sClient, cfg.InformerResyncPeriod)
			lbcfFactory := externalversions.NewSharedInformerFactory(lbcfClient, cfg.InformerResyncPeriod)
			c := lbcfcontroller.NewController(
				cfg,
				k8sClient,
				lbcfClient,
				k8sFactory.Core().V1().Pods(),
				k8sFactory.Core().V1().Services(),
				lbcfFactory.Lbcf().V1beta1().LoadBalancers(),
				lbcfFactory.Lbcf().V1beta1().LoadBalancerDrivers(),
				lbcfFactory.Lbcf().V1beta1().BackendGroups(),
				lbcfFactory.Lbcf().V1beta1().BackendRecords())
			c.Start()
			k8sFactory.Start(wait.NeverStop)
			lbcfFactory.Start(wait.NeverStop)
			lbcfcontroller.NewAdmitServer(c, cfg.ServerCrt, cfg.ServerKey).Start()
			<-wait.NeverStop
		},
	}
	cfg.AddFlags(cmd.Flags())
	return cmd
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
