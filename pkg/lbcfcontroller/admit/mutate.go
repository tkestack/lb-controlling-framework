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

package admit

import (
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

func addLBNameLabel(createLabel, isReplace bool, key string, value string) Patch {
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

func addDefaultSvcProtocol() Patch {
	return Patch{
		OP:    patchOpAdd,
		Path:  "/spec/service/port/protocol",
		Value: "TCP",
	}
}

func addDefaultPodProtocol() Patch {
	return Patch{
		OP:    patchOpAdd,
		Path:  "/spec/pods/port/protocol",
		Value: "TCP",
	}
}
