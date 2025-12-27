# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

## Location to install build tools
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.18.0
# ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
# ENVTEST_K8S_VERSION is the Kubernetes version to download for envtest (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
CRD_OUTPUT_DIR ?= ../../crds
CRD_MODULE_LABEL ?= gpu-control-plane
CRD_FILES := $(CRD_OUTPUT_DIR)/gpu.deckhouse.io_physicalgpus.yaml \
	$(CRD_OUTPUT_DIR)/gpu.deckhouse.io_gpuclasses.yaml

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: manifests
manifests: controller-gen ## Generate CRDs.
	$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=$(CRD_OUTPUT_DIR)
	@$(MAKE) crd-labels

.PHONY: crd-labels
crd-labels: ## Ensure module label exists on generated CRDs.
	@for f in $(CRD_FILES); do \
		if [ -f "$$f" ]; then \
			if ! grep -q "module: $(CRD_MODULE_LABEL)" "$$f"; then \
				if grep -q "^  labels:" "$$f"; then \
					awk 'BEGIN{added=0} /^  labels:/ {print; if(!added){print "    module: $(CRD_MODULE_LABEL)"; added=1} next} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
				else \
					awk 'BEGIN{added=0} /^metadata:/ && !added {print; print "  labels:\n    module: $(CRD_MODULE_LABEL)"; added=1; next} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
				fi; \
			fi; \
		fi; \
	done

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: build
build: manifests generate fmt vet ## Build all binaries locally.
	mkdir -p bin
	go build -o bin/gpu-controller ./cmd/gpu-controller
	go build -o bin/gpu-node-agent ./cmd/gpu-node-agent
	go build -o bin/gpu-dra-controller ./cmd/gpu-dra-controller
	go build -o bin/gpu-handler ./cmd/gpu-handler

.PHONY: run
run: manifests generate fmt vet ## Run gpu-controller locally.
	go run ./cmd/gpu-controller

## Tooling
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
	\techo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
	\texit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
