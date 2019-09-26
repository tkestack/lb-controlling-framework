SHELL := /bin/bash

BINARY = lbcf-controller
OUTPUT_PATH ?= output/$(BINARY)
LBCF_IMAGE = lbcf-controller

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
mkfile_dir := $(dir $(mkfile_path))

.PHONY: docker-build
docker-build:
	cd $(mkfile_dir) && go mod vendor && \
	docker run --rm -v "$(mkfile_dir)":/go/src/git.code.oa.com/tkestack/lb-controlling-framework \
	-w /go/src/git.code.oa.com/tkestack/lb-controlling-framework \
	-e GO111MODULE=off \
	golang:1.12 \
	make build

.PHONY: build
build:
	go build -o $(OUTPUT_PATH) git.code.oa.com/tkestack/lb-controlling-framework/cmd/lbcf-controller

.PHONY: image
image:
	make docker-build && \
	cd $(mkfile_dir) && \
	cp build/docker/Dockerfile output/ && \
	docker build -t $(LBCF_IMAGE) output/ && \
	docker save $(LBCF_IMAGE) -o output/lbcf-controller-image.tar
