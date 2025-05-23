
# Image URL to use all building/pushing image targets
IMG ?= quay.io/konveyor/oadp-non-admin:latest
# Kubernetes version from OpenShift 4.19.x https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/#4-stable
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.32

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=non-admin-controller-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

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
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e:
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build --load -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	@if [ -d "config/crd" ]; then \
		$(KUSTOMIZE) build config/crd > dist/install.yaml; \
	fi
	echo "---" >> dist/install.yaml  # Add a document separator before appending
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default >> dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.16.5
ENVTEST_VERSION ?= v0.0.0-20250308055145-5fe7bb3edc86
GOLANGCI_LINT_VERSION ?= v2.0.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef

##@ oadp-nac specifics

## Tool Binaries
OC_CLI ?= $(shell which oc)
EC ?= $(LOCALBIN)/ec-$(EC_VERSION)

## Tool Versions
EC_VERSION ?= 2.8.0

.PHONY: editorconfig
editorconfig: $(LOCALBIN) ## Download editorconfig locally if necessary.
	@[ -f $(EC) ] || { \
	set -e ;\
	ec_binary=ec-$(shell go env GOOS)-$(shell go env GOARCH) ;\
	ec_tar=$(LOCALBIN)/$${ec_binary}.tar.gz ;\
	curl -sSLo $${ec_tar} https://github.com/editorconfig-checker/editorconfig-checker/releases/download/$(EC_VERSION)/$${ec_binary}.tar.gz ;\
	tar xzf $${ec_tar} ;\
	rm -rf $${ec_tar} ;\
	mv $(LOCALBIN)/$${ec_binary} $(EC) ;\
	}

COVERAGE_THRESHOLD=60

.PHONY: ci
ci: simulation-test lint docker-build hadolint check-generate check-manifests ec check-go-dependencies check-images ## Run all project continuous integration (CI) checks locally.

.PHONY: simulation-test
simulation-test: envtest ## Run unit and integration tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $(shell go list ./... | grep -v oadp-non-admin/test) -test.coverprofile cover.out -test.v -ginkgo.vv
	@make check-coverage

.PHONY: check-coverage
check-coverage: ## Check if test coverage threshold was reached.
	@{ \
	set -e ;\
	current_coverage=$(shell go tool cover -func=cover.out | grep total | grep -Eo "[0-9]+\.[0-9]+") ;\
	if  [ "$$(echo "$$current_coverage < $(COVERAGE_THRESHOLD)" | bc -l)" -eq 1 ];then \
		echo "Current coverage ($$current_coverage%) is below project threshold of $(COVERAGE_THRESHOLD)%" ;\
		exit 1 ;\
	fi ;\
	echo "Coverage threshold of $(COVERAGE_THRESHOLD)% reached: $$current_coverage%" ;\
	}

.PHONY: hadolint
hadolint: ## Run container file linter.
	$(CONTAINER_TOOL) run --rm --interactive ghcr.io/hadolint/hadolint < ./Dockerfile

.PHONY: check-generate
check-generate: generate ## Check if 'make generate' was run.
	test -z "$(shell git status --short)" || (echo "run 'make generate' to generate code" && exit 1)

.PHONY: check-manifests
check-manifests: manifests ## Check if 'make manifests' was run.
	test -z "$(shell git status --short)" || (echo "run 'make manifests' to generate code" && exit 1)

.PHONY: ec
ec: editorconfig ## Run file formatter checks against all project's files.
	$(EC)

.PHONY: go-dependencies
go-dependencies: ## Update go dependencies.
	go mod tidy
	go mod verify

.PHONY: check-go-dependencies
check-go-dependencies: go-dependencies ## Check if 'make go-dependencies' was run.
	test -z "$(shell git status --short)" || (echo "run 'make go-dependencies' to update go dependencies" && exit 1)

.PHONY: check-images
check-images: MANAGER_IMAGE:=$(shell grep -I 'newName: ' ./config/manager/kustomization.yaml | awk -F': ' '{print $$2}')
check-images: MANAGER_TAG:=$(shell grep -I 'newTag: ' ./config/manager/kustomization.yaml | awk -F': ' '{print $$2}')
check-images: ## Check if images are the same in Makefile and config/manager/kustomization.yaml
	@if [ "$(MANAGER_IMAGE)" == "" ];then echo "No manager image found" && exit 1;fi
	@if [ "$(MANAGER_TAG)" == "" ];then echo "No manager tag found" && exit 1;fi
	@grep -Iq "IMG ?= $(MANAGER_IMAGE):$(MANAGER_TAG)" ./Makefile || (echo "Images differ" && exit 1)

.PHONY: update-velero-manifests
update-velero-manifests: ## Update Velero manifests used by NAC, from OADP_OPERATOR_PATH
ifeq ($(OADP_OPERATOR_PATH),)
	$(error You must set OADP_OPERATOR_PATH to run this command)
endif
	@{ \
	NAC_VELERO_VERSION=$(shell grep -I 'github.com/openshift/velero' $(shell pwd)/go.mod | awk -F' ' '{print $$5}') ;\
	OADP_VELERO_VERSION=$(shell grep -I 'github.com/openshift/velero' $(OADP_OPERATOR_PATH)/go.mod | awk -F' ' '{print $$5}') ;\
	sed -i "s%$$NAC_VELERO_VERSION%$$OADP_VELERO_VERSION%" $(shell pwd)/go.mod ;\
	for file_name in $(shell ls $(shell pwd)/hack/extra-crds);do \
		cp $(OADP_OPERATOR_PATH)/config/crd/bases/$$file_name $(shell pwd)/hack/extra-crds/$$file_name && \
		sed -i "1s%^%# Code generated by make update-velero-manifests. DO NOT EDIT.\n%" $(shell pwd)/hack/extra-crds/$$file_name;done ;\
	}

.PHONY: docker-buildx-nac
docker-buildx-nac: ## Build and push docker image for the manager for cross-platform support
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile .
