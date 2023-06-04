GOOS ?= $(shell go env GOOS)

# Git information
GIT_VERSION ?= $(shell git describe --tags --always)
GIT_COMMIT_HASH ?= $(shell git rev-parse HEAD)
GIT_TREESTATE = "clean"
GIT_DIFF = $(shell git diff --quiet >/dev/null 2>&1; if [ $$? -eq 1 ]; then echo "1"; fi)
ifeq ($(GIT_DIFF), 1)
    GIT_TREESTATE = "dirty"
endif
BUILDDATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS = ""

# Images management
REGISTRY ?= docker.io
REGISTRY_NAMESPACE?= 2456868764
REGISTRY_USER_NAME?=""
REGISTRY_PASSWORD?=""

# Image URL to use all building/pushing image targets
HTTPBIN_IMG ?= "${REGISTRY}/${REGISTRY_NAMESPACE}/httpbin:${GIT_VERSION}"

HTTPBIN_SIDECAR_IMG ?= "${REGISTRY}/${REGISTRY_NAMESPACE}/httpbin:sidecar"

## docker buildx support platform
PLATFORMS ?= linux/arm64,linux/amd64

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)


## Tool Binaries
SWAGGER ?= $(LOCALBIN)/swag
GOLANG_LINT ?= $(LOCALBIN)/golangci-lint
GOFUMPT  ?= $(LOCALBIN)/gofumpt


## Tool Versions
SWAGGER_VERSION ?= v1.16.1
GOLANG_LINT_VERSION ?= v1.52.2
GOFUMPT_VERSION ?= latest


# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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

.PHONY: swagger
swagger: install-swagger## Generate swagger docs.
	$(SWAGGER) init --parseDependency -d cmd/,pkg/ -o cmd/docs
	@rm -f cmd/docs/docs.go cmd/docs/swagger.yaml

.PHONY: fmt
fmt: install-gofumpt ## Run gofumpt against code.
	$(GOFUMPT) -l -w .

.PHONY: vet
vet: ## Run go vet against code.
	@find . -type f -name '*.go'| grep -v "/vendor/" | xargs gofmt -w -s

# Run mod tidy against code
.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: lint
lint: install-golangci-lint  ## Run golang lint against code
	GO111MODULE=on $(GOLANG_LINT) run ./... --timeout=30m -v  --disable-all --enable=gofumpt --enable=govet --enable=staticcheck --enable=ineffassign --enable=misspell

.PHONY: test
test: fmt vet  ## Run all tests.
	go test -coverprofile coverage.out -covermode=atomic ./...


.PHONY: echoLDFLAGS
echoLDFLAGS:
	@echo $(LDFLAGS)


.PHONY: all
all:  test build

.PHONY: build
build: ## Build binary with the httpbin.
	CGO_ENABLED=0 GOOS=$(GOOS) go build -ldflags $(LDFLAGS) -o bin/httpbin cmd/main.go


.PHONY: image
image: ## Build docker image with the httpbin.
	docker build --build-arg LDFLAGS=$(LDFLAGS) --build-arg PKGNAME=httpbin -t ${HTTPBIN_IMG} .

.PHONY: image-sidecar
image-sidecar: ## Build docker image with the httpbin.
	docker build --build-arg LDFLAGS=$(LDFLAGS) --build-arg PKGNAME=httpbin -t ${HTTPBIN_SIDECAR_IMG} ./Dockerfile_sidecar

.PHONY: push-image
push-image: ## Push images.
ifneq ($(REGISTRY_USER_NAME), "")
	docker login -u $(REGISTRY_USER_NAME) -p $(REGISTRY_PASSWORD) ${REGISTRY}
endif
	docker push ${HTTPBIN_IMG}


.PHONY: push-image-sidecar
push-image-sidecar: ## Push images.
ifneq ($(REGISTRY_USER_NAME), "")
	docker login -u $(REGISTRY_USER_NAME) -p $(REGISTRY_PASSWORD) ${REGISTRY}
endif
	docker push ${HTTPBIN_SIDECAR_IMG}


.PHONY: image-buildx
image-buildx:  ## Build and push docker image for the httpbin for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name project-httpbin-builder
	docker buildx use project-httpbin-builder
	- docker buildx build --build-arg LDFLAGS=$(LDFLAGS) --build-arg PKGNAME=httpbin  --push --platform=$(PLATFORMS) --tag ${HTTPBIN_IMG} -f Dockerfile.cross .
	- docker buildx rm project-httpbin-builder
	rm Dockerfile.cross


.PHONY: install-swagger
install-swagger: $(LOCALBIN) ## Download swagger locally if necessary.
	test -s $(LOCALBIN)/swag  || \
	GOBIN=$(LOCALBIN) go install  github.com/swaggo/swag/cmd/swag@$(SWAGGER_VERSION)


.PHONY: install-golangci-lint
install-golangci-lint: $(LOCALBIN) ## Download golangci lint locally if necessary.
	test -s $(LOCALBIN)/golangci-lint  && $(LOCALBIN)/golangci-lint --version | grep -q $(GOLANG_LINT_VERSION) || \
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANG_LINT_VERSION)


.PHONY: install-gofumpt
install-gofumpt: $(LOCALBIN) ## Download gofumpt locally if necessary.
	test -s $(LOCALBIN)/gofumpt || \
	GOBIN=$(LOCALBIN) go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
