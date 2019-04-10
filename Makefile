SHELL := /bin/bash

BINARY = lbcf-controller
OUTPUT_PATH ?= output/$(BINARY)

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
mkfile_dir := $(dir $(mkfile_path))

.PHONY: docker-build
docker-build:
	cd $(mkfile_dir) && go mod vendor && \
	docker run --rm -v "$(mkfile_dir)":/go/src/git.tencent.com/tke/lb-controlling-framework \
	-w /go/src/git.tencent.com/tke/lb-controlling-framework \
	-e GO111MODULE=off \
	golang:1.12 \
	make build

.PHONY: build
build:
	go build -o $(OUTPUT_PATH) git.tencent.com/tke/lb-controlling-framework/lbcf-controller/cmd

