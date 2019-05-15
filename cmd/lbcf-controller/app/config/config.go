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

package config

import (
	"github.com/spf13/pflag"
	"time"
)

type Config struct {
	InformerResyncPeriod time.Duration
	MinRetryDelay        time.Duration
	KubeConfig           string
	ServerCrt            string
	ServerKey            string
}

func NewConfig() *Config {
	return &Config{}
}

func (o *Config) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&o.InformerResyncPeriod, "informer-resync-period", 1*time.Minute, "resync period for informers")
	fs.DurationVar(&o.MinRetryDelay, "min-retry-delay", 1*time.Minute, "minimum retry delay for failed webhook calls")
	fs.StringVar(&o.KubeConfig, "kubeconfig", "", "Path to kubeconfig file with authorization information")
	fs.StringVar(&o.ServerCrt, "server-crt", "/etc/lbcf/server.crt", "Path to crt file for admit webhook server")
	fs.StringVar(&o.ServerKey, "server-key", "/etc/lbcf/server.key", "Path to key file for admit webhook server")
}
