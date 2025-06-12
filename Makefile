# Application version encoded in all the binaries.
VERSION = 0.0.0

# Base go module name.
MODULE := $(shell cat go.mod | grep -m1 module | awk '{print $$2}')

# Git revision.
REVISION := $(shell git rev-parse HEAD)

# Commands to build, the first lot are architecture agnostic and will be built
# for your host's architecture.  The latter are going to run in Kubernetes, so
# want to be amd64.
CONTROLLERS = \
  unikorn-cluster-manager-controller \
  unikorn-cluster-controller\
  unikorn-virtualcluster-controller\
  unikorn-server \
  unikorn-monitor

# Release will do cross compliation of all images for the 'all' target.
# Note we aren't fucking about with docker here because that opens up a
# whole can of worms to do with caching modules and pisses on performance,
# primarily making me rage.  For image creation, this, by necessity,
# REQUIRES multiarch images to be pushed to a remote registry because
# Docker apparently cannot support this after some 3 years...  So don't
# run that target locally when compiling in release mode.
ifdef RELEASE
CONTROLLER_ARCH := amd64 arm64
BUILDX_OUTPUT := --push
else
CONTROLLER_ARCH := $(shell go env GOARCH)
BUILDX_OUTPUT := --load
endif

# Calculate the platform list to pass to docker buildx.
BUILDX_PLATFORMS := $(shell echo $(patsubst %,linux/%,$(CONTROLLER_ARCH)) | sed 's/ /,/g')

# Some constants to describe the repository.
BINDIR = bin
CMDDIR = cmd
SRCDIR = src
GENDIR = generated
CRDDIR = charts/kubernetes/crds

# Where to install things.
PREFIX = $(HOME)/bin

# List of binaries to build.
CONTROLLER_BINARIES := $(foreach arch,$(CONTROLLER_ARCH),$(foreach ctrl,$(CONTROLLERS),$(BINDIR)/$(arch)-linux-gnu/$(ctrl)))

# List of sources to trigger a build.
# TODO: Bazel may be quicker, but it's a massive hog, and a pain in the arse.
SOURCES := $(shell find . -type f -name *.go) go.mod go.sum

# Source files defining custom resource APIs
APISRC = $(shell find pkg/apis -name [^z]*.go -type f)

# Some bits about go.
GOPATH := $(shell go env GOPATH)
GOBIN := $(if $(shell go env GOBIN),$(shell go env GOBIN),$(GOPATH)/bin)

# Common linker flags.
FLAGS=-trimpath -ldflags '-X $(MODULE)/pkg/constants.Version=$(VERSION) -X $(MODULE)/pkg/constants.Revision=$(REVISION)'

# Defines the linter version.
LINT_VERSION=v2.1.5

# Defines the version of the CRD generation tools to use.
CONTROLLER_TOOLS_VERSION=v0.17.3

# Defines the version of code generator tools to use.
# This should be kept in sync with the Kubenetes library versions defined in go.mod.
CODEGEN_VERSION := $(shell grep k8s.io/apimachinery go.mod | awk '{ print $$2; }')

OPENAPI_CODEGEN_VERSION=v2.4.1
OPENAPI_CODEGEN_FLAGS=-package openapi -config pkg/openapi/config.yaml
OPENAPI_SCHEMA=pkg/openapi/server.spec.yaml
OPENAPI_FILES = \
	pkg/openapi/types.go \
	pkg/openapi/schema.go \
	pkg/openapi/client.go \
	pkg/openapi/router.go

MOCKGEN_VERSION=v0.3.0

# This is the base directory to generate kubernetes API primitives from e.g.
# clients and CRDs.
GENAPIBASE = github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1

# These are generic arguments that need to be passed to client generation.
GENARGS = --go-header-file hack/boilerplate.go.txt

# This defines how docker containers are tagged.
DOCKER_ORG = ghcr.io/nscaledev

# Main target, builds all binaries.
.PHONY: all
all: $(CONTROLLER_BINARIES) $(CRDDIR)

# Create a binary output directory, this should be an order-only prerequisite.
$(BINDIR) $(BINDIR)/amd64-linux-gnu $(BINDIR)/arm64-linux-gnu:
	mkdir -p $@

$(BINDIR)/amd64-linux-gnu/%: $(SOURCES) $(GENDIR) $(OPENAPI_FILES) | $(BINDIR)/amd64-linux-gnu
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(FLAGS) -o $@ $(CMDDIR)/$*/main.go

$(BINDIR)/arm64-linux-gnu/%: $(SOURCES) $(GENDIR) | $(BINDIR)/arm64-linux-gnu
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(FLAGS) -o $@ $(CMDDIR)/$*/main.go

# TODO: we may wamt to consider porting the rest of the CRD and client generation
# stuff over... that said, we don't need the clients really do we, controller-runtime
# does all the magic for us.
.PHONY: generate
generate:
	@go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)
	go generate ./...

# Create container images.  Use buildkit here, as it's the future, and it does
# good things, like per file .dockerignores and all that jazz.
.PHONY: images
images: $(CONTROLLER_BINARIES)
	if [ -n "$(RELEASE)" ]; then docker buildx create --name unikorn --use; fi
	for image in ${CONTROLLERS}; do docker buildx build --platform $(BUILDX_PLATFORMS) $(BUILDX_OUTPUT) -f docker/$${image}/Dockerfile -t ${DOCKER_ORG}/$${image}:${VERSION} .; done;
	if [ -n "$(RELEASE)" ]; then docker buildx rm unikorn; fi

# Purely lazy command that builds and pushes to docker hub.
.PHONY: images-push
images-push: images
	for image in ${CONTROLLERS}; do docker push ${DOCKER_ORG}/$${image}:${VERSION}; done

.PHONY: images-kind-load
images-kind-load: images
	for image in ${CONTROLLERS}; do kind load docker-image ${DOCKER_ORG}/$${image}:${VERSION}; done

.PHONY: test-unit
test-unit:
	go test -coverpkg ./... -coverprofile cover.out ./...
	go tool cover -html cover.out -o cover.html

# Build a binary and install it.
$(PREFIX)/%: $(BINDIR)/%
	install -m 750 $< $@

# Create any CRDs defined into the target directory.
$(CRDDIR): $(APISRC)
	@mkdir -p $@
	@go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
	$(GOBIN)/controller-gen crd:crdVersions=v1 paths=./pkg/apis/unikorn/... output:dir=$@
	@touch $(CRDDIR)

# Generate a clientset to interact with our custom resources.
$(GENDIR): $(APISRC)
	@go install k8s.io/code-generator/cmd/deepcopy-gen@$(CODEGEN_VERSION)
	$(GOBIN)/deepcopy-gen --output-file zz_generated.deepcopy.go $(GENARGS) $(GENAPIBASE)
	@touch $@

# Generate the server schema, types and router boilerplate.
pkg/openapi/types.go: $(OPENAPI_SCHEMA)
	@go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OPENAPI_CODEGEN_VERSION)
	oapi-codegen -generate types $(OPENAPI_CODEGEN_FLAGS) -o $@ $<

pkg/openapi/schema.go: $(OPENAPI_SCHEMA)
	@go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OPENAPI_CODEGEN_VERSION)
	oapi-codegen -generate spec $(OPENAPI_CODEGEN_FLAGS) -o $@ $<

pkg/openapi/client.go: $(OPENAPI_SCHEMA)
	@go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OPENAPI_CODEGEN_VERSION)
	oapi-codegen -generate client $(OPENAPI_CODEGEN_FLAGS) -o $@ $<

pkg/openapi/router.go: $(OPENAPI_SCHEMA)
	@go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OPENAPI_CODEGEN_VERSION)
	oapi-codegen -generate chi-server $(OPENAPI_CODEGEN_FLAGS) -o $@ $<

# When checking out, the files timestamps are pretty much random, and make cause
# spurious rebuilds of generated content.  Call this to prevent that.
.PHONY: touch
touch:
	touch $(CRDDIR) $(GENDIR) pkg/apis/unikorn/v1alpha1/zz_generated.deepcopy.go

# Perform linting.
# This must pass or you will be denied by CI.
.PHOMY: lint
lint: $(GENDIR)
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION)
	$(GOBIN)/golangci-lint run --timeout=10m ./...
	helm lint --strict charts/kubernetes

# Validate the server OpenAPI schema is legit.
.PHONY: validate
validate: $(OPENAPI_FILES)
	go run ./hack/validate_openapi

# Validate the docs can be generated without fail.
.PHONY: validate-docs
validate-docs: $(OPENAPI_FILES)
	go run github.com/unikorn-cloud/core/hack/docs --dry-run

# Perform license checking.
# This must pass or you will be denied by CI.
.PHONY: license
license:
	go run github.com/unikorn-cloud/core/hack/check_license
