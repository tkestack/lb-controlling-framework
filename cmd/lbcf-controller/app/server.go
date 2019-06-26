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
	"flag"
	"k8s.io/klog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"git.code.oa.com/k8s/lb-controlling-framework/cmd/lbcf-controller/app/config"
	"git.code.oa.com/k8s/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller/admission"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
)

func NewServer() *cobra.Command {
	cfg := config.NewConfig()

	cmd := &cobra.Command{
		Use: "lbcf-controller",

		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.NewContext(cfg)
			admissionWebhookServer := admission.NewWebhookServer(ctx, cfg.ServerCrt, cfg.ServerKey)
			lbcf := lbcfcontroller.NewController(ctx)

			ctx.Start()
			admissionWebhookServer.Start()
			lbcf.Start()

			mux := http.NewServeMux()
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("ok"))
			})
			go http.ListenAndServe(":11029", mux)

			<-wait.NeverStop
		},
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(fs)
	cfg.AddFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		panic(err)
	}
	cmd.Flags().AddGoFlagSet(fs)
	return cmd
}
