SHELL := /bin/bash
export SHELLOPTS:=$(if $(SHELLOPTS),$(SHELLOPTS):)pipefail:errexit

IMAGE_TAG = garm-agent-build
IMAGE_BUILDER=$(shell (which docker || which podman))
IS_PODMAN=$(shell (($(IMAGE_BUILDER) --version | grep -q podman) && echo "yes" || echo "no"))
GARM_REF ?= $(shell git rev-parse --abbrev-ref HEAD)
USER_ID=$(if $(filter yes,$(IS_PODMAN)),0,$(shell id -u))
USER_GROUP=$(if $(filter yes,$(IS_PODMAN)),0,$(shell id -g))

ifeq ($(IS_PODMAN),yes)
    EXTRA_ARGS := -v /etc/ssl/certs/ca-certificates.crt:/etc/ssl/certs/ca-certificates.crt
endif

.ONESHELL:

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


default: build-static

.PHONY : build-static

build-static: ## Build garm-agent statically
	@echo Building garm-agent
	$(IMAGE_BUILDER) build $(EXTRA_ARGS) --tag $(IMAGE_TAG) -f Dockerfile.build-static .
	mkdir -p build
	$(IMAGE_BUILDER) run --rm -e USER_ID=$(USER_ID) -e GARM_REF=$(GARM_REF) -e USER_GROUP=$(USER_GROUP) -v $(PWD)/build:/build/output:z $(IMAGE_TAG) /build-static.sh
	@echo Binaries are available in $(PWD)/build

test:
	go test -race -count=1 -v ./...

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

## Tool Versions
GOLANGCI_LINT_VERSION ?= v1.64.8

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary. If wrong version is installed, it will be overwritten.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint && $(LOCALBIN)/golangci-lint --version | grep -q $(GOLANGCI_LINT_VERSION) || \
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

##@ Lint / Verify
.PHONY: lint
lint: golangci-lint $(GOLANGCI_LINT) ## Run linting.
	$(GOLANGCI_LINT) run -v --build-tags=testing,integration $(GOLANGCI_LINT_EXTRA_ARGS)

.PHONY: lint-fix
lint-fix: golangci-lint $(GOLANGCI_LINT) ## Lint the codebase and run auto-fixers if supported by the linte
	GOLANGCI_LINT_EXTRA_ARGS=--fix $(MAKE) lint