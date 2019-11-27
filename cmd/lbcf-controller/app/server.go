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

package app

import (
	goflag "flag"
	"k8s.io/klog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/config"
	"tkestack.io/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/admission"
	"tkestack.io/lb-controlling-framework/pkg/version"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
)

func NewServer() *cobra.Command {
	rootCmd := &cobra.Command{
		Use: "lbcr-controller",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	rootCmd.AddCommand(newCmdStart())
	rootCmd.AddCommand(version.NewCmdVersion())

	fs := goflag.NewFlagSet(os.Args[0], goflag.ExitOnError)
	klog.InitFlags(fs)
	rootCmd.PersistentFlags().AddGoFlagSet(fs)

	return rootCmd
}

func newCmdStart() *cobra.Command {
	cfg := config.NewConfig()

	cmd := &cobra.Command{
		Use: "run",
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
	cfg.AddFlags(cmd.LocalFlags())
	return cmd
}
