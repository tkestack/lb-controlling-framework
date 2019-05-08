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
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func TestValidate_OperationCreate(t *testing.T) {
	var createCnt, updateCnt, deleteCnt int
	createFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		createCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	updateFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		updateCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	deleteFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		deleteCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID:       types.UID("12345"),
			Operation: v1beta1.Create,
		},
	}
	resp := validate(ar, createFunc, updateFunc, deleteFunc)
	if !resp.Response.Allowed {
		t.Fatalf("expect allow")
	} else if resp.Response.UID != "12345" {
		t.Fatalf("expect uid 12345, get %v", resp.Response.UID)
	} else if updateCnt != 0 || deleteCnt != 0 {
		t.Fatalf("updateCnt: %d, deleteCnt: %d", updateCnt, deleteCnt)
	} else if createCnt != 1 {
		t.Fatalf("createCnt: %d", createCnt)
	}
}

func TestValidate_OperationUpdate(t *testing.T) {
	var createCnt, updateCnt, deleteCnt int
	createFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		createCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	updateFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		updateCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	deleteFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		deleteCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID:       types.UID("12345"),
			Operation: v1beta1.Update,
		},
	}
	resp := validate(ar, createFunc, updateFunc, deleteFunc)
	if !resp.Response.Allowed {
		t.Fatalf("expect allow")
	} else if resp.Response.UID != "12345" {
		t.Fatalf("expect uid 12345, get %v", resp.Response.UID)
	} else if createCnt != 0 || deleteCnt != 0 {
		t.Fatalf("createCnt: %d, deleteCnt: %d", createCnt, deleteCnt)
	} else if updateCnt != 1 {
		t.Fatalf("updateCnt: %d", updateCnt)
	}
}

func TestValidate_OperationDelete(t *testing.T) {
	var createCnt, updateCnt, deleteCnt int
	createFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		createCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	updateFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		updateCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	deleteFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		deleteCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID:       types.UID("12345"),
			Operation: v1beta1.Delete,
		},
	}
	resp := validate(ar, createFunc, updateFunc, deleteFunc)
	if !resp.Response.Allowed {
		t.Fatalf("expect allow")
	} else if resp.Response.UID != "12345" {
		t.Fatalf("expect uid 12345, get %v", resp.Response.UID)
	} else if createCnt != 0 || updateCnt != 0 {
		t.Fatalf("createCnt: %d, updateCnt: %d", createCnt, updateCnt)
	} else if deleteCnt != 1 {
		t.Fatalf("deleteCnt: %d", deleteCnt)
	}
}

func TestValidate_OperationConnect(t *testing.T) {
	var createCnt, updateCnt, deleteCnt int
	createFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		createCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	updateFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		updateCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	deleteFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		deleteCnt++
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID:       types.UID("12345"),
			Operation: v1beta1.Connect,
		},
	}
	resp := validate(ar, createFunc, updateFunc, deleteFunc)
	if !resp.Response.Allowed {
		t.Fatalf("expect allow")
	} else if resp.Response.UID != "12345" {
		t.Fatalf("expect uid 12345, get %v", resp.Response.UID)
	} else if createCnt != 0 || updateCnt != 0 || deleteCnt != 0 {
		t.Fatalf("createCnt: %d, updateCnt: %d, deleteCnt: %d", createCnt, updateCnt, deleteCnt)
	}
}

func TestMutate(t *testing.T) {
	mutateFunc := func(*v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	ar := &v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID:       types.UID("12345"),
			Operation: v1beta1.Delete,
		},
	}
	resp := mutate(ar, mutateFunc)
	if !resp.Response.Allowed {
		t.Fatalf("expect allow")
	} else if resp.Response.UID != "12345" {
		t.Fatalf("expect uid 12345, get %v", resp.Response.UID)
	}
}
