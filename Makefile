# Copyright 2025 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

SHELL := /bin/bash

ROOT := $(CURDIR)
CONTROLLER_DIR := $(ROOT)/src/controller
GOMODCACHE := $(ROOT)/.cache/gomod
BIN_DIR := $(ROOT)/.bin

export PATH := $(BIN_DIR):$(PATH)

GO ?= go
GOFLAGS ?= -count=1
GOLANGCI_LINT_VERSION ?= 1.64.8
MODULE_SDK_VERSION ?= 0.3.7
DMT_VERSION ?= 0.1.44

GOLANGCI_LINT ?= $(BIN_DIR)/golangci-lint
MODULE_SDK ?= $(BIN_DIR)/module-sdk
DMT ?= $(BIN_DIR)/dmt
WERF ?= werf

export GOMODCACHE

.PHONY: ensure-bin-dir ensure-golangci-lint ensure-module-sdk ensure-dmt ensure-tools \
	fmt tidy controller-build controller-test hooks-test lint-go lint-docs lint-dmt \
	lint test verify clean cache docs werf-build kubeconform e2e

ensure-bin-dir:
	@mkdir -p $(BIN_DIR)

ensure-golangci-lint: ensure-bin-dir
	@INSTALL_DIR=$(BIN_DIR) GOLANGCI_LINT_VERSION=$(GOLANGCI_LINT_VERSION) GOLANGCI_LINT_USE_GO_INSTALL=1 GOLANGCI_LINT_TOOLCHAIN=go1.25.0 ./tools/install-golangci-lint.sh

ensure-module-sdk: ensure-bin-dir
	@INSTALL_DIR=$(BIN_DIR) MODULE_SDK_VERSION=$(MODULE_SDK_VERSION) ./tools/install-module-sdk.sh

ensure-dmt: ensure-bin-dir
	@INSTALL_DIR=$(BIN_DIR) DMT_VERSION=$(DMT_VERSION) ./tools/install-dmt.sh

ensure-tools: ensure-golangci-lint ensure-dmt

cache:
	@mkdir -p $(GOMODCACHE)

fmt:
	@echo "==> gofmt"
	@gofmt -w $(shell git ls-files '*.go')

tidy: cache
	@echo "==> go mod tidy (controller)"
	@cd $(CONTROLLER_DIR) && $(GO) mod tidy

controller-build: cache
	@echo "==> go build (controller)"
	@mkdir -p $(BIN_DIR)
	@cd $(CONTROLLER_DIR) && $(GO) build -o $(BIN_DIR)/gpu-control-plane-controller ./cmd/main.go

controller-test: cache
	@echo "==> go test (controller)"
	@cd $(CONTROLLER_DIR) && $(GO) test $(GOFLAGS) ./...

hooks-test: cache
	@echo "==> go test (hooks)"
	@cd images/hooks && $(GO) test $(GOFLAGS) ./...

lint-docs:
	@echo "==> prettier (markdown)"
	@docker run --rm \
		-v $(ROOT):/work ghcr.io/deckhouse/virtualization/prettier:3.2.5 \
		sh -c 'cd /work && prettier -c "**/*.md"'

lint-go: cache ensure-golangci-lint
	@echo "==> golangci-lint"
	@(cd $(CONTROLLER_DIR) && $(GOLANGCI_LINT) run ./...)
	@echo "==> go vet"
	@cd $(CONTROLLER_DIR) && $(GO) vet ./...

lint-dmt: ensure-dmt
	@echo "==> dmt lint"
	@$(DMT) lint

lint: lint-go lint-docs lint-dmt

test: controller-test hooks-test

verify: lint test

clean:
	@rm -rf $(GOMODCACHE)

docs:
	@./tools/render-docs.py

werf-build: ensure-module-sdk
	@echo "==> werf build"
	@$(WERF) build $(if $(MODULES_MODULE_SOURCE),--repo "$(MODULES_MODULE_SOURCE)") $(if $(MODULES_MODULE_TAG),--tag "$(MODULES_MODULE_TAG)")

kubeconform:
	@echo "==> kubeconform"
	@cd tools/kubeconform && ./kubeconform.sh

e2e:
	@echo "==> e2e (kind or target cluster)"
	@./tools/dev/kind-e2e.sh
