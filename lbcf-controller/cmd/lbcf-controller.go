package main

import (
	"fmt"
	"time"

	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
)

func main() {
	fmt.Println("hello world")
	cfg, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatal(err)
	}
	client := versioned.NewForConfigOrDie(cfg)
	client.LbcfV1beta1().LoadBalancers("demo").Get("hello", v1.GetOptions{})

	factory := externalversions.NewSharedInformerFactory(client, 10*time.Second)
	go factory.Lbcf().V1beta1().LoadBalancers().Informer().Run(wait.NeverStop)
	go factory.Lbcf().V1beta1().BackendGroups().Informer().Run(wait.NeverStop)
}
