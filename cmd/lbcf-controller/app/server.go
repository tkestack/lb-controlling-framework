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
	"net/http"

	"git.code.oa.com/k8s/lb-controlling-framework/cmd/lbcf-controller/app/config"
	"git.code.oa.com/k8s/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller"
	"git.code.oa.com/k8s/lb-controlling-framework/pkg/lbcfcontroller/admit"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
)

func NewServer() *cobra.Command {
	cfg := config.NewConfig()

	cmd := &cobra.Command{
		Use: "lbcf-controller",

		Run: func(cmd *cobra.Command, args []string) {
			context := context.NewContext(cfg)
			admitServer := admit.NewAdmitServer(context, cfg.ServerCrt, cfg.ServerKey)
			lbcf := lbcfcontroller.NewController(context)

			context.Start()
			admitServer.Start()
			lbcf.Start()

			mux := http.NewServeMux()
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("ok"))
			})
			go http.ListenAndServe(":11029", mux)

			<-wait.NeverStop
		},
	}
	cfg.AddFlags(cmd.Flags())
	return cmd
}
