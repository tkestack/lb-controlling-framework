/*
 * Tencent is pleased to support the open source community by making TKEStack
 * available.
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

// Code generated by client-gen. DO NOT EDIT.

package v1beta1

import (
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
	v1beta1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned/scheme"
)

type LbcfV1beta1Interface interface {
	RESTClient() rest.Interface
	BackendGroupsGetter
	BackendRecordsGetter
	LoadBalancersGetter
	LoadBalancerDriversGetter
}

// LbcfV1beta1Client is used to interact with features provided by the lbcf.tkestack.io group.
type LbcfV1beta1Client struct {
	restClient rest.Interface
}

func (c *LbcfV1beta1Client) BackendGroups(namespace string) BackendGroupInterface {
	return newBackendGroups(c, namespace)
}

func (c *LbcfV1beta1Client) BackendRecords(namespace string) BackendRecordInterface {
	return newBackendRecords(c, namespace)
}

func (c *LbcfV1beta1Client) LoadBalancers(namespace string) LoadBalancerInterface {
	return newLoadBalancers(c, namespace)
}

func (c *LbcfV1beta1Client) LoadBalancerDrivers(namespace string) LoadBalancerDriverInterface {
	return newLoadBalancerDrivers(c, namespace)
}

// NewForConfig creates a new LbcfV1beta1Client for the given config.
func NewForConfig(c *rest.Config) (*LbcfV1beta1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &LbcfV1beta1Client{client}, nil
}

// NewForConfigOrDie creates a new LbcfV1beta1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *LbcfV1beta1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new LbcfV1beta1Client for the given RESTClient.
func New(c rest.Interface) *LbcfV1beta1Client {
	return &LbcfV1beta1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1beta1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *LbcfV1beta1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}