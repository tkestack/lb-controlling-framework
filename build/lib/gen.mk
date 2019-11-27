# Tencent is pleased to support the open source community by making TKEStack
# available.
#
# Copyright (C) 2012-2019 Tencent. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use
# this file except in compliance with the License. You may obtain a copy of the
# License at
#
# https://opensource.org/licenses/Apache-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OF ANY KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations under the License.

# set the kubernetes apimachinery package dir
K8S_APIMACHINERY_DIR = $(shell go list -f '{{ .Dir }}' -m k8s.io/apimachinery)
# set the kubernetes api package dir
K8S_API_DIR = $(shell go list -f '{{ .Dir }}' -m k8s.io/api)
# set the gogo protobuf package dir
GOGO_PROTOBUF_DIR = $(shell go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)

.PHONY: gen.run
gen.run: gen.clean gen.generator gen.api

# ==============================================================================
# Generator

.PHONY: gen.generator
gen.generator:
	@echo "===========> Building code generator $(VERSION) docker image"
	@$(DOCKER) build --pull -t $(REGISTRY_PREFIX)/code-generator:$(VERSION) -f $(ROOT_DIR)/build/docker/tools/code-generator/Dockerfile $(ROOT_DIR)/build/docker/tools/code-generator

.PHONY: gen.api
gen.api:
	$(eval CODE_GENERATOR_VERSION := $(shell $(DOCKER) images --filter 'reference=$(REGISTRY_PREFIX)/code-generator' |sed 's/[ ][ ]*/,/g' |cut -d ',' -f 2 |sed -n '2p'))
	@$(DOCKER) run --rm \
		-v $(ROOT_DIR):/go/src/$(ROOT_PACKAGE) \
	 	$(REGISTRY_PREFIX)/code-generator:$(CODE_GENERATOR_VERSION) \
	 	/root/code.sh \
	 	deepcopy-external,client-external,informer-external,lister-external\
	 	$(ROOT_PACKAGE)/pkg/client-go \
	 	$(ROOT_PACKAGE)/pkg/apis \
	 	$(ROOT_PACKAGE)/pkg/apis \
	 	"lbcf.tkestack.io:v1beta1"

.PHONY: gen.clean
gen.clean:
	@rm -rf $(ROOT_DIR)/pkg/client-go/clientset
	@rm -rf $(ROOT_DIR)/pkg/client-go/informers
	@rm -rf $(ROOT_DIR)/pkg/client-go/listers
	@find . -type f -name 'generated.*' -delete
	@find . -type f -name 'zz_generated*.go' -delete
	@find . -type f -name 'types_swagger_doc_generated.go' -delete

