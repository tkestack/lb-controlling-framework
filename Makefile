SHELL := /bin/bash

BINARY = lbcf-controller
OUTPUT_PATH ?= output/$(BINARY)

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
mkfile_dir := $(dir $(mkfile_path))

.PHONY: docker-build
docker-build:
	docker run --rm -v "$(mkfile_dir)":/usr/src/git.tencent.com/lb-controlling-framework -w /usr/src/git.tencent.com/lb-controlling-framework golang:1.12 make build

.PHONY: build
build:
	go build -o $(OUTPUT_PATH) git.tencent.com/lb-controlling-framework/cmd

