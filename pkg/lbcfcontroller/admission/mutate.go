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

package admission

import (
	lbcfapi "git.code.oa.com/k8s/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	"path"
	"strings"
)

const (
	patchOpAdd     = "add"
	patchOpReplace = "replace"
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

func defaultSvcProtocol() Patch {
	return Patch{
		OP:    patchOpAdd,
		Path:  "/spec/service/port/protocol",
		Value: "TCP",
	}
}

func defaultPodProtocol() Patch {
	return Patch{
		OP:    patchOpAdd,
		Path:  "/spec/pods/port/protocol",
		Value: "TCP",
	}
}

type backendGroupPatch struct {
	obj     *lbcfapi.BackendGroup
	patches []Patch
}

func (bp *backendGroupPatch) addLabel() {
	var skip, createLabel, replace bool
	if bp.obj.Labels != nil {
		if value, ok := bp.obj.Labels[lbcfapi.LabelLBName]; ok && value == bp.obj.Spec.LBName {
			skip = true
		} else if ok {
			replace = true
		}
	}
	createLabel = bp.obj.Labels == nil || len(bp.obj.Labels) == 0
	if !skip {
		bp.patches = append(bp.patches, addLabel(createLabel, replace, lbcfapi.LabelLBName, bp.obj.Spec.LBName))
	}
}

func (bp *backendGroupPatch) setDefaultProtocol() {
	if bp.obj.Spec.Service != nil && bp.obj.Spec.Service.Port.Protocol == "" {
		bp.patches = append(bp.patches, defaultSvcProtocol())
	} else if bp.obj.Spec.Pods != nil && bp.obj.Spec.Pods.Port.Protocol == "" {
		bp.patches = append(bp.patches, defaultPodProtocol())
	}
}

func (bp *backendGroupPatch) patch() []Patch {
	return bp.patches
}
