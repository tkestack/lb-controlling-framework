package app

import (
	"time"

	lbcfclientset "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/informers/externalversions"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

func NewServer() *cobra.Command {
	opts := newOptions()

	cmd := &cobra.Command{
		Use: "lbcf-controller",

		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := rest.InClusterConfig()
			if err != nil {
				klog.Fatal(err)
			}

			k8sClient := kubernetes.NewForConfigOrDie(cfg)
			lbcfClient := lbcfclientset.NewForConfigOrDie(cfg)
			k8sFactory := informers.NewSharedInformerFactory(k8sClient, opts.ResyncPeriod)
			lbcfFactory := externalversions.NewSharedInformerFactory(lbcfClient, opts.ResyncPeriod)
			c := lbcfcontroller.NewController(
				opts,
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
			lbcfcontroller.NewAdmitServer(c).Start()
		},
	}
	opts.addFlags(cmd.Flags())
	return cmd
}

type Options struct {
	ResyncPeriod time.Duration
}

func newOptions() *Options {
	return &Options{}
}

func (o *Options) addFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&o.ResyncPeriod, "resync-period", 10*time.Second, "resync period for informers")
}
