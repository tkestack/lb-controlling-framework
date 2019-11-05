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

package config

import (
	"flag"
	"time"
)

type Config struct {
	InformerResyncPeriod time.Duration
	MinRetryDelay        time.Duration
	RetryDelayStep       time.Duration
	MaxRetryDelay        time.Duration
	KubeConfig           string
	ServerCrt            string
	ServerKey            string
}

func NewConfig() *Config {
	return &Config{}
}

func (o *Config) AddFlags(fs *flag.FlagSet) {
	fs.DurationVar(&o.InformerResyncPeriod,
		"informer-resync-period", 1*time.Minute, "resync period for informers")
	fs.DurationVar(&o.MinRetryDelay,
		"min-retry-delay", 5*time.Second, "minimum retry delay for failed webhook calls")
	fs.DurationVar(&o.RetryDelayStep,
		"retry-delay-step", 10*time.Second, "the value added to retry delay for each webhook failure")
	fs.DurationVar(&o.MaxRetryDelay,
		"max-retry-delay", 2*time.Minute, "maximum retry delay for failed webhook calls")
	fs.StringVar(&o.KubeConfig,
		"kubeconfig", "", "Path to kubeconfig file with authorization information")
	fs.StringVar(&o.ServerCrt,
		"server-crt", "/etc/lbcf/server.crt", "Path to crt file for admit webhook server")
	fs.StringVar(&o.ServerKey,
		"server-key", "/etc/lbcf/server.key", "Path to key file for admit webhook server")
}
