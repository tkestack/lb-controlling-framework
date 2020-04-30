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

package admission

import (
	"fmt"
	"path"
	"strings"
	"time"

	lbcfapi "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	patchOpAdd     = "add"
	patchOpReplace = "replace"
	patchOpRemove  = "remove"
)

// Patch is the json patch struct
type Patch struct {
	OP    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func addLabel(createLabel, isReplace bool, key string, value string) Patch {
	patch := Patch{}
	if createLabel {
		patch.OP = patchOpAdd
		patch.Path = path.Join("/", "metadata", "labels")
		patch.Value = map[string]string{
			key: value,
		}
		return patch
	}

	key = strings.ReplaceAll(key, "~", "~0")
	key = strings.ReplaceAll(key, "/", "~1")
	patch.Path = path.Join("/", "metadata", "labels", key)
	patch.Value = value
	if isReplace {
		patch.OP = patchOpReplace
	} else {
		patch.OP = patchOpAdd
	}
	return patch
}

func addFinalizer(createFinalizer bool, finalizer string) Patch {
	patch := Patch{
		OP: patchOpAdd,
	}
	if createFinalizer {
		patch.Path = path.Join("/", "metadata", "finalizers")
		patch.Value = []string{finalizer}
		return patch
	}

	patch.Path = path.Join("/", "metadata", "finalizers", "-")
	patch.Value = finalizer
	return patch
}

type backendGroupPatch struct {
	obj     *lbcfapi.BackendGroup
	patches []Patch
}

// addLabel add labels on user applied BackendGroup.
// Suppose an user applied a BackendGroup as follows:
// apiVersion: lbcf.tkestack.io/v1beta1
// kind: BackendGroup
// metadata:
//   labels:
//     example.com/my-label: my-value
//   name: test-example-bg
// spec:
//   loadBalancers:
//   - lb1
//   - lb2
//   pods:
//     byLabel:
//       selector:
//         k8s-app: deploy-pod
//     port:
//       portNumber: 80
//       protocol: TCP
// the mutated labels are:
// labels:
//   example.com/my-label: my-value
//   lbcf.tkestack.io/lb-name: lb1
//   name.lb.lbcf.tkestack.io/lb1: True
//   name.lb.lbcf.tkestack.io/lb2: True
func (bp *backendGroupPatch) addLabel() {
	// In LBCF v1.1.1 and before, one BackendGroup can only have one LoadBalancer,
	// so BackendGroups are labeled in the following format: lbcf.tkestack.io/lb-name:${lbName}
	// Since we decided to allow one BackendGroup connected to multiple LoadBalancers,
	// the label on BackendGroup is changed to: name.lb.lbcf.tkestack.io/${lbName}:True
	labels := make(map[string]string)
	if bp.obj.Spec.LBName != nil {
		labels[lbcfapi.LabelLBName] = *bp.obj.Spec.LBName
		labels[lbcfapi.LabelLBNamePrefix+*bp.obj.Spec.LBName] = "True"
	}
	for i, lb := range bp.obj.Spec.LoadBalancers {
		if i == 0 {
			labels[lbcfapi.LabelLBName] = lb
		}
		labels[lbcfapi.LabelLBNamePrefix+lb] = "True"
	}
	createLabel := len(bp.obj.Labels) == 0
	if createLabel {
		// create object metadata.labels
		patch := Patch{}
		patch.OP = patchOpAdd
		patch.Path = path.Join("/", "metadata", "labels")
		patchValue := make(map[string]string)
		for k, v := range labels {
			patchValue[k] = v
		}
		patch.Value = patchValue
		bp.patches = append(bp.patches, patch)
		return
	} else {
		// add or replace key-value to metadata.labels
		for k, v := range labels {
			keyInPath := strings.Replace(k, "/", "~1", -1)
			// add key-value if the key doesn't exist
			if value, ok := bp.obj.Labels[k]; !ok {
				bp.patches = append(bp.patches, Patch{
					OP:    patchOpAdd,
					Path:  path.Join("/", "metadata", "labels", keyInPath),
					Value: v,
				})
			} else if ok && v != value {
				// replace the value if the key is already exist
				bp.patches = append(bp.patches, Patch{
					OP:    patchOpReplace,
					Path:  path.Join("/", "metadata", "labels", keyInPath),
					Value: v,
				})
			}
		}
	}
}

func (bp *backendGroupPatch) convertPortSelector() {
	if bp.obj.Spec.Pods != nil {
		var portNeedMerge *lbcfapi.PortSelector
		if oldPort := bp.obj.Spec.Pods.Port; oldPort != nil {
			// delete spec.pods.port
			bp.patches = append(bp.patches, Patch{
				OP:   patchOpRemove,
				Path: "/spec/pods/port",
			})
			// determine if pods.port need to be merged with pods.ports
			oldPortProtocol := "TCP"
			if oldPort.Protocol != "" {
				oldPortProtocol = oldPort.Protocol
			}
			found := false
			for _, newPort := range bp.obj.Spec.Pods.Ports {
				newPortProtocol := "TCP"
				if newPort.Protocol != "" {
					newPortProtocol = newPort.Protocol
				}
				if oldPort.GetPort() == newPort.GetPort() && oldPortProtocol == newPortProtocol {
					found = true
					break
				}
			}
			if !found {
				portNeedMerge = bp.obj.Spec.Pods.Port
			}
		}

		needCreate := len(bp.obj.Spec.Pods.Ports) == 0
		if needCreate {
			// create array spec.pods.ports if it doesn't exist
			if portNeedMerge == nil {
				return
			}
			protocol := "TCP"
			if portNeedMerge.Protocol != "" {
				protocol = portNeedMerge.Protocol
			}
			bp.patches = append(bp.patches, Patch{
				OP:   patchOpAdd,
				Path: "/spec/pods/ports",
				Value: []lbcfapi.PortSelector{
					{
						Port:     portNeedMerge.GetPort(),
						Protocol: protocol,
					},
				},
			})
			return
		} else {
			// add spec.pods.port as an element of spec.pods.ports
			if portNeedMerge != nil {
				protocol := "TCP"
				if portNeedMerge.Protocol != "" {
					protocol = portNeedMerge.Protocol
				}
				bp.patches = append(bp.patches, Patch{
					OP:   patchOpAdd,
					Path: "/spec/pods/ports/-",
					Value: lbcfapi.PortSelector{
						Port:     portNeedMerge.GetPort(),
						Protocol: protocol,
					},
				})
			}
			// convert all portNumber to port
			for i, p := range bp.obj.Spec.Pods.Ports {
				// set default protocol
				if p.Protocol == "" {
					bp.patches = append(bp.patches, Patch{
						OP:    patchOpAdd,
						Path:  fmt.Sprintf("/spec/pods/ports/%d/protocol", i),
						Value: "TCP",
					})
				}
				// substitute port for portNumber
				if p.PortNumber != nil {
					bp.patches = append(bp.patches, Patch{
						OP:   patchOpRemove,
						Path: fmt.Sprintf("/spec/pods/ports/%d/portNumber", i),
					})
				}
				if p.Port == 0 {
					bp.patches = append(bp.patches, Patch{
						OP:    patchOpAdd,
						Path:  fmt.Sprintf("/spec/pods/ports/%d/port", i),
						Value: *p.PortNumber,
					})
				}
			}
		}
	}

	if bp.obj.Spec.Service != nil {
		// delete spec.service.port.portNumber
		if bp.obj.Spec.Service.Port.PortNumber != nil {
			bp.patches = append(bp.patches, Patch{
				OP:   patchOpRemove,
				Path: "/spec/service/port/portNumber",
			})
			// substitute port for portNumber
			if bp.obj.Spec.Service.Port.Port == 0 {
				bp.patches = append(bp.patches, Patch{
					OP:    patchOpAdd,
					Path:  "/spec/service/port/port",
					Value: *bp.obj.Spec.Service.Port.PortNumber,
				})
			}
		}
		// set default protocol
		if bp.obj.Spec.Service.Port.Protocol == "" {
			bp.patches = append(bp.patches, Patch{
				OP:    patchOpAdd,
				Path:  "/spec/service/port/protocol",
				Value: "TCP",
			})
		}
	}
}

func (bp *backendGroupPatch) convertLoadBalancers() {
	if bp.obj.Spec.LBName == nil {
		return
	}
	// delete spec.lbName
	bp.patches = append(bp.patches, Patch{
		OP:   patchOpRemove,
		Path: "/spec/lbName",
	})
	// if spec.lbName is included in spec.loadBalancers, return
	for _, lbName := range bp.obj.Spec.LoadBalancers {
		if lbName == *bp.obj.Spec.LBName {
			return
		}
	}
	needCreate := len(bp.obj.Spec.LoadBalancers) == 0
	if needCreate {
		// create array LoadBalancer if it doesn't exist
		bp.patches = append(bp.patches,
			Patch{
				OP:   patchOpAdd,
				Path: "/spec/loadBalancers",
				Value: []string{
					*bp.obj.Spec.LBName,
				},
			},
		)
	} else {
		// add spec.lbName as an element of spec.loadBalancers
		bp.patches = append(bp.patches, Patch{
			OP:    patchOpAdd,
			Path:  "/spec/loadBalancers/-",
			Value: *bp.obj.Spec.LBName,
		})
	}
}

func (bp *backendGroupPatch) setDefaultDeregisterPolicy() {
	if bp.obj.Spec.DeregisterPolicy != nil {
		return
	}
	bp.patches = append(bp.patches, Patch{
		OP:    patchOpAdd,
		Path:  "/spec/deregisterPolicy",
		Value: lbcfapi.DeregisterIfNotReady,
	})
}

func (bp *backendGroupPatch) patch() []Patch {
	return bp.patches
}

const (
	defaultWebhookTimeout = 10 * time.Second
)

type driverPatch struct {
	obj     *lbcfapi.LoadBalancerDriver
	patches []Patch
}

func (dp *driverPatch) setWebhook() {
	createArray := len(dp.obj.Spec.Webhooks) == 0
	if createArray {
		dp.patches = append(dp.patches, Patch{
			OP:    patchOpAdd,
			Path:  path.Join("/", "spec", "webhooks"),
			Value: []interface{}{},
		})
	}

	existWebhooks := sets.NewString()
	for _, has := range dp.obj.Spec.Webhooks {
		existWebhooks.Insert(has.Name)
	}

	for known := range webhooks.KnownWebhooks {
		if existWebhooks.Has(known) {
			continue
		}
		dp.patches = append(dp.patches, Patch{
			OP:   patchOpAdd,
			Path: path.Join("/", "spec", "webhooks", "-"),
			Value: lbcfapi.WebhookConfig{
				Name: known,
				Timeout: lbcfapi.Duration{
					Duration: defaultWebhookTimeout,
				},
			},
		})
	}
}

func (dp *driverPatch) patch() []Patch {
	return dp.patches
}
