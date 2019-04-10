package main

import (
	"fmt"

	"git.tencent.com/lb-controlling-framework/pkg/client-go/clientset/versioned"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
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
}
