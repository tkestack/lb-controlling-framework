package app

import (
	"time"

	lbcfclientset "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions/lbcf.tke.cloud.tencent.com/v1beta1"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

func NewServer() *cobra.Command {
	opts := newOptions()

	cmd := &cobra.Command{
		Use: "lbcf-controller",

		Run: func(cmd *cobra.Command, args []string) {
			podInformer, lbInformer, lbDriverInformer, bgInformer, brInformer := opts.initInformers()
			startLBCFController(
				podInformer,
				lbInformer,
				lbDriverInformer,
				bgInformer,
				brInformer,
			)
		},
	}
	opts.addFlags(cmd.Flags())
	return cmd
}

func startLBCFController(
	podInformer v1.PodInformer,
	lbInformer v1beta1.LoadBalancerInformer,
	lbDriverInformer v1beta1.LoadBalancerDriverInformer,
	bgInformer v1beta1.BackendGroupInformer,
	brInformer v1beta1.BackendRecordInformer,
) {
	// TODO: add eventHandlers to informers

	go lbcfcontroller.NewController().Run()
}

type options struct {
	resyncPeriod time.Duration
}

func newOptions() *options {
	return &options{}
}

func (o *options) addFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&o.resyncPeriod, "resync-period", 10*time.Second, "resync period for informers")
}

func (o *options) initInformers() (
	podInformer v1.PodInformer,
	lbInformer v1beta1.LoadBalancerInformer,
	lbDriverInformer v1beta1.LoadBalancerDriverInformer,
	bgInformer v1beta1.BackendGroupInformer,
	brInformer v1beta1.BackendRecordInformer) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatal(err)
	}

	podInformer = informers.NewSharedInformerFactory(kubernetes.NewForConfigOrDie(cfg), o.resyncPeriod).Core().V1().Pods()

	lbcfFactory := externalversions.NewSharedInformerFactory(lbcfclientset.NewForConfigOrDie(cfg), o.resyncPeriod)
	lbInformer = lbcfFactory.Lbcf().V1beta1().LoadBalancers()
	bgInformer = lbcfFactory.Lbcf().V1beta1().BackendGroups()
	lbDriverInformer = lbcfFactory.Lbcf().V1beta1().LoadBalancerDrivers()
	brInformer = lbcfFactory.Lbcf().V1beta1().BackendRecords()
	return
}
