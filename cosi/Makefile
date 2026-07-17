.DEFAULT_GOAL := build
.SUFFIXES: # remove legacy builtin suffixes to allow easier make debugging
SHELL = /usr/bin/env bash

# If GOARCH is not set in the env, find it
GOARCH ?= $(shell go env GOARCH)

## ==== ARGS =====
DOCKER            ?= docker                    # Container build tool compatible with `docker` API
PLATFORM          ?= linux/$(GOARCH)           # Platform for 'build'
BUILD_ARGS        ?=                           # Additional args for 'build'
SAMPLE_DRIVER_TAG ?= infinidat-cosi-driver:latest # Image tag for controller image build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: clean
clean:
	rm -rf $(LOCALBIN)

.PHONY: all
all: build

##@ Image

.PHONY: build
build: ## Build only the controller container image
	go build -o cosi/bin/infinidat-cosi-driver cosi/cmd/main.go
