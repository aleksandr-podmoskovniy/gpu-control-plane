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
CONTROLLER_DIR := $(ROOT)/images/gpu-control-plane-artifact
KUBE_API_REWRITER_DIR := $(ROOT)/images/kube-api-rewriter
GFD_EXTENDER_DIR := $(ROOT)/images/gfd-extender
GPU_ARTIFACT_DIR := $(ROOT)/images/gpu-artifact
GOMODCACHE := $(ROOT)/.cache/gomod
BIN_DIR := $(ROOT)/.bin
COVERAGE_DIR := $(ROOT)/artifacts/coverage
export PATH := $(BIN_DIR):$(PATH)

GO ?= go
GOFLAGS ?= -count=1
GOLANGCI_LINT_VERSION ?= 2.6.2
MODULE_SDK_VERSION ?= 0.5.0
DMT_VERSION ?= 0.1.63
GOLANGCI_LINT_OPTS ?= --timeout=5m
DEADCODE_VERSION ?= latest
PRETTIER_VERSION ?= 3.2.5

GOLANGCI_LINT ?= $(BIN_DIR)/golangci-lint
MODULE_SDK ?= $(BIN_DIR)/module-sdk
DMT ?= $(BIN_DIR)/dmt
DEADCODE ?= $(BIN_DIR)/deadcode
WERF ?= werf
WERF_PLATFORM ?= linux/amd64

export GOMODCACHE

.PHONY: ensure-bin-dir ensure-golangci-lint ensure-module-sdk ensure-dmt ensure-deadcode ensure-tools \
	fmt tidy controller-build controller-test hooks-test rewriter-test gfd-extender-test lint-go lint-docs lint-dmt \
	lint test verify clean cache docs werf-build kubeconform helm-template deadcode e2e

ensure-bin-dir:
	@mkdir -p $(BIN_DIR)

ensure-golangci-lint: ensure-bin-dir
	@INSTALL_DIR=$(BIN_DIR) GOLANGCI_LINT_VERSION=$(GOLANGCI_LINT_VERSION) GOLANGCI_LINT_USE_GO_INSTALL=1 GOLANGCI_LINT_TOOLCHAIN=go1.25.0 ./tools/install-golangci-lint.sh

ensure-module-sdk: ensure-bin-dir
	@INSTALL_DIR=$(BIN_DIR) MODULE_SDK_VERSION=$(MODULE_SDK_VERSION) ./tools/install-module-sdk.sh

ensure-dmt: ensure-bin-dir
	@INSTALL_DIR=$(BIN_DIR) DMT_VERSION=$(DMT_VERSION) ./tools/install-dmt.sh

ensure-deadcode: ensure-bin-dir cache
	@echo "==> deadcode"
	@GOBIN=$(BIN_DIR) $(GO) install golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

ensure-tools: ensure-golangci-lint ensure-module-sdk ensure-dmt ensure-deadcode

cache:
	@mkdir -p $(GOMODCACHE)

coverage-dir:
	@mkdir -p $(COVERAGE_DIR)

fmt:
	@echo "==> gofmt"
	@gofmt -w $(shell git ls-files '*.go')

tidy: cache
	@echo "==> go mod tidy (controller)"
	@cd $(CONTROLLER_DIR) && $(GO) mod tidy

controller-build: cache
	@echo "==> go build (controller)"
	@mkdir -p $(BIN_DIR)
	@cd $(CONTROLLER_DIR) && $(GO) build -o $(BIN_DIR)/gpu-control-plane-controller ./cmd/gpu-control-plane-controller

controller-test: cache coverage-dir
	@echo "==> go test (controller)"
	@cd $(CONTROLLER_DIR) && $(GO) test $(GOFLAGS) -coverprofile $(COVERAGE_DIR)/controller.out ./...

hooks-test: cache coverage-dir
	@echo "==> go test (hooks)"
	@cd images/hooks && $(GO) test $(GOFLAGS) -coverprofile $(COVERAGE_DIR)/hooks.out ./...

rewriter-test: cache coverage-dir
	@echo "==> go test (kube-api-rewriter)"
	@cd $(KUBE_API_REWRITER_DIR) && $(GO) test $(GOFLAGS) -coverprofile $(COVERAGE_DIR)/kube-api-rewriter.out ./...

gfd-extender-test: cache coverage-dir
	@echo "==> go test (gfd-extender)"
	@cd $(GFD_EXTENDER_DIR) && CGO_ENABLED=0 $(GO) test $(GOFLAGS) -tags=nonvml -coverprofile $(COVERAGE_DIR)/gfd-extender.out ./...

lint-docs:
	@echo "==> prettier (markdown)"
	@if docker info >/dev/null 2>&1; then \
		docker run --rm -v $(ROOT):/work ghcr.io/deckhouse/virtualization/prettier:$(PRETTIER_VERSION) sh -c 'cd /work && prettier -c "**/*.md"'; \
	else \
		echo "docker is unavailable; running prettier via npx"; \
		cd $(ROOT) && npx --yes prettier@$(PRETTIER_VERSION) -c '**/*.md'; \
	fi

lint-go: cache ensure-golangci-lint
	@echo "==> golangci-lint"
	@(cd $(CONTROLLER_DIR) && $(GOLANGCI_LINT) run $(GOLANGCI_LINT_OPTS) ./...)
	@echo "==> go vet"
	@cd $(CONTROLLER_DIR) && $(GO) vet ./...

lint-dmt: ensure-dmt
	@echo "==> dmt lint"
	@$(DMT) lint

lint: lint-go lint-docs lint-dmt

test: controller-test hooks-test rewriter-test gfd-extender-test

verify: lint test deadcode helm-template kubeconform

clean:
	@rm -rf $(GOMODCACHE)

docs:
	@./tools/render-docs.py

werf-build: ensure-module-sdk
	@echo "==> werf build"
	@$(WERF) build --platform=$(WERF_PLATFORM) $(if $(MODULES_MODULE_SOURCE),--repo "$(MODULES_MODULE_SOURCE)") $(if $(MODULES_MODULE_TAG),--add-custom-tag "%image%-$(MODULES_MODULE_TAG)")

kubeconform:
	@echo "==> kubeconform"
	@cd tools/kubeconform && ./kubeconform.sh

helm-template:
	@echo "==> helm template (bootstrap state fixtures)"
	@cd tools/helm-tests && ./helm-template.sh

deadcode: ensure-deadcode
	@echo "==> deadcode (controller)"
	@cd $(CONTROLLER_DIR) && $(DEADCODE) -test ./...
	@echo "==> deadcode (hooks)"
	@cd $(ROOT)/images/hooks && $(DEADCODE) -test ./...
	@echo "==> deadcode (gpu-artifact)"
	@cd $(GPU_ARTIFACT_DIR) && $(DEADCODE) -test ./...
	@echo "==> deadcode (kube-api-rewriter)"
	@cd $(KUBE_API_REWRITER_DIR) && $(DEADCODE) -test ./...
	@echo "==> deadcode (pre-delete-hook)"
	@cd $(ROOT)/images/pre-delete-hook && $(DEADCODE) -test ./...
	@echo "==> deadcode (gfd-extender, linux-only)"
	@cd $(GFD_EXTENDER_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(DEADCODE) -test ./...

e2e:
	@echo "==> e2e (kind or target cluster)"
	@./tools/dev/kind-e2e.sh
