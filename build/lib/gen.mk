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

# Determine api packages
API_DIR ?= $(wildcard ${ROOT_DIR}/pkg/apis/lbcf.tkestack.io/*)
# Determine api versions
APIS_VERSIONS ?= $(foreach api,${API_DIR},$(notdir ${api}))
# Determine api group versions
API_GROUP_VERSIONS ?= $(foreach api_version,$(APIS_VERSIONS),lbcf.tkestack.io:$(api_version))
# set the code-generator image version
CODE_GENERATOR_VERSION = v1.17.0-3

.PHONY: gen.run
gen.run: gen.clean gen.generator gen.api

# ==============================================================================
# Generator

.PHONY: gen.generator
gen.generator:
	@echo "===========> Building code generator $(VERSION) docker image"
	@$(DOCKER) pull $(REGISTRY_PREFIX)/code-generator:$(CODE_GENERATOR_VERSION)

.PHONY: gen.api
gen.api:
	@$(DOCKER) run --rm \
		-v $(ROOT_DIR):/go/src/$(ROOT_PACKAGE) \
	 	$(REGISTRY_PREFIX)/code-generator:$(CODE_GENERATOR_VERSION) \
	 	bash -x /root/code.sh \
	 	deepcopy-external,defaulter-external,client-external,lister-external,informer-external\
	 	$(ROOT_PACKAGE)/pkg/client-go \
	 	$(ROOT_PACKAGE)/pkg/apis \
	 	$(ROOT_PACKAGE)/pkg/apis \
	  	"$(API_GROUP_VERSIONS)"

.PHONY: gen.clean
gen.clean:
	@rm -rf $(ROOT_DIR)/pkg/client-go/clientset
	@rm -rf $(ROOT_DIR)/pkg/client-go/informers
	@rm -rf $(ROOT_DIR)/pkg/client-go/listers
	@find . -type f -name 'generated.*' -delete
	@find . -type f -name 'zz_generated*.go' -delete
	@find . -type f -name 'types_swagger_doc_generated.go' -delete

