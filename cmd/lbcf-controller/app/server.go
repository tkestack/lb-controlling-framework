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
	"git.tencent.com/tke/lb-controlling-framework/cmd/lbcf-controller/app/context"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller"
	"git.tencent.com/tke/lb-controlling-framework/pkg/lbcfcontroller/admit"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
)

func NewServer() *cobra.Command {
	cfg := config.NewConfig()

	cmd := &cobra.Command{
		Use: "lbcf-controller",

		Run: func(cmd *cobra.Command, args []string) {
			context := context.NewContext(cfg)
			context.Start()
			context.WaitForCacheSync()

			admit.NewAdmitServer(context, cfg.ServerCrt, cfg.ServerKey).Start()
			lbcfcontroller.NewController(context).Start()

			<-wait.NeverStop
		},
	}
	cfg.AddFlags(cmd.Flags())
	return cmd
}
