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

.PHONY: all
all: lint test build

# ==============================================================================
# Build options

ROOT_PACKAGE=tkestack.io/lb-controlling-framework
VERSION_PACKAGE=tkestack.io/lb-controlling-framework/pkg/version

# ==============================================================================
# Includes

include build/lib/common.mk
include build/lib/golang.mk
include build/lib/image.mk

# ==============================================================================
# Usage

define USAGE_OPTIONS

Options:
  DEBUG        Whether to generate debug symbols. Default is 0.
  IMAGES       Backend images to make. All by default.
  PLATFORMS    The platform to build. Default is host platform and arch.
  BINS         The binaries to build. Default is all of cmd.
  VERSION      The version information compiled into binaries.
               The default is obtained from git.
  V            Set to 1 enable verbose build. Default is 0.
endef
export USAGE_OPTIONS

# ==============================================================================
# Targets

## build: Build source code for host platform.
.PHONY: build
build:
	@$(MAKE) go.build

## build.all: Build source code for all platforms.
.PHONY: build.all
build.all:
	@$(MAKE) go.build.all

## image: Build docker images.
.PHONY: image
image:
	@$(MAKE) image.build

## push: Build docker images and push to registry.
.PHONY: push
push:
	@$(MAKE) image.push

.PHONY: help
help: Makefile
	@echo -e "\nUsage: make <TARGETS> <OPTIONS> ...\n\nTargets:"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo "$$USAGE_OPTIONS"
