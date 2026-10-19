# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0

green := \033[1;32m[+]
reset := \033[0m
OS             := $(shell go env GOOS)
ARCH           := $(shell go env GOARCH)

## Auto Detect Platform
ifeq ($(PLATFORM),)
	ifneq ($(filter arm%,$(ARCH)),)
		PLATFORM = linux/arm64
	else
		PLATFORM = linux/amd64
	endif
endif

## Version setting
VERSION ?= $(shell sed -n 's/^version:[[:space:]]*//p' VERSION.yaml)

BINARY         := rootstock
OS             := $(shell go env GOOS)
ARCH           := $(shell go env GOARCH)

## Version setting
# Single source of truth: VERSION.yaml via yq, with a git-based fallback for
# environments without yq. The version package accepts a leading
# <major>.<minor>.<patch> followed by an arbitrary suffix, so the "0.0.0-<sha>"
# fallback and CI's "<version>-pr-<n>" are both valid.
VERSION ?= $(shell \
		command -v yq > /dev/null 2>&1 && yq -r '.version' VERSION.yaml \
		|| echo "0.0.0-$$(git rev-parse --short HEAD)" \
)

# Build toolchain knobs. Overridable so the Dockerfile can cross-compile and
# enforce FIPS, e.g. make build GOOS=linux GOARCH=arm64 GOFIPS140=certified.
GOOS       ?= $(OS)
GOARCH     ?= $(ARCH)
CGO_ENABLED ?= 0
GOFIPS140  ?=

# Integration test mockup server
# Requires pdu-mockup data at testdata/pdu-mockup (already added to the repo).
# Or override the path: make test-integration PDU_MOCKUP_PATH=/your/path/to/pdu-mockup

PDU_MOCKUP_PATH  ?= $(CURDIR)/testdata/pdu-mockup
MOCKUP_PORT      ?= 8000
MOCKUP_CONTAINER := redfish-mockup




# Container image / local Kubernetes (kind) settings
IMAGE          ?= ghcr.io/cloudoperators/rootstock:dev-$(USER)
# The IMAGE repository/tag split and all other kind lifecycle logic live in
# hack/kind/control.sh, which the kind-* targets delegate to.
KIND_CLUSTER   		?= rootstock-local
CHART_DIR      		:= k8s_configuration/chart
CHART_RELEASE  		?= rootstock
CHART_NAMESPACE 	?= rootstock
REDFISH_SECRET 		?= rootstock-redfish
REDFISH_PASSWORD 	?= admin


.PHONY: build test test-unit test-integration test-contract test-performance \
				mockup-start mockup-stop \
				lint lint-go lint-md lint-shell \
				scan \
				docker-build \
        kind-up kind-cluster kind-prepare kind-deploy kind-verify kind-cleanup kind-down \
        helm-lint helm-template \
				generate generate-changelog generate-version \
				get-version test-changelog \
				help

# Build

# Build metadata stamped into the binary via -ldflags. Each is overridable so
# the Dockerfile can inject host-computed values (the container build context
# excludes .git, so git must not be shelled out from inside the image).
GIT_COMMIT     ?= $(shell git rev-parse HEAD 2>/dev/null)
GIT_TREE_STATE ?= $(shell if [ -z "$$(git status --porcelain 2>/dev/null)" ]; then echo clean; else echo dirty; fi)
BUILD_DATE     ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

build: LDFLAGS    = "-s -w \
							-X main.version=$(VERSION) \
							-X main.gitCommit=$(GIT_COMMIT) \
							-X main.gitTreeState=$(GIT_TREE_STATE) \
							-X main.buildDate=$(BUILD_DATE)"
build: ## Compile a static server binary to bin/
	@echo "$(green) building binary$(reset)"
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) GOFIPS140=$(GOFIPS140) \
		go build \
			-installsuffix 'static' \
			-ldflags $(LDFLAGS) \
			-o bin/$(BINARY) \
			.

# Tests

test-unit: ## Run unit tests with race detection and coverage
	@echo "$(green) unit testing$(reset)"
	CGO_ENABLED=1 go test ./... -race -parallel=10 -timeout=10m \
		-covermode=atomic \
		-coverprofile=coverage.out
	go tool cover -func=coverage.out -o=coverage.out

test: test-unit ## Alias for test-unit

test-integration: mockup-start ## Start mockup server, run integration tests, stop server
	@echo "$(green) integration testing$(reset)"
	go test ./... -tags=integration -parallel=10 -timeout=10m -v; STATUS=$$?; \
	$(MAKE) mockup-stop; \
	exit $$STATUS

test-contract: ## Run contract tests (TODO: not yet implemented)
	@echo "TODO: contract tests not yet implemented"

test-performance: ## Run performance tests (TODO: not yet implemented)
	@echo "TODO: performance tests not yet implemented"
	@echo "TODO: will cover queue length and concurrent API request limits"

# ---------------------------------------------------------------------------
# Mockup server lifecycle (used by test-integration)
# ---------------------------------------------------------------------------

mockup-start: ## Start the DMTF Redfish mockup server in Docker
	@PDU_MOCKUP_PATH=$(PDU_MOCKUP_PATH) MOCKUP_PORT=$(MOCKUP_PORT) MOCKUP_CONTAINER=$(MOCKUP_CONTAINER) \
		hack/mockup-start.sh

mockup-stop: ## Stop the DMTF Redfish mockup server
	@MOCKUP_CONTAINER=$(MOCKUP_CONTAINER) hack/mockup-stop.sh

# Code quality
lint-go: ## Run golangci-lint on Go code
	golangci-lint run ./... --fix

lint-md: ## Run prettier for markdown files
	npx --yes prettier --prose-wrap always --write "**/*.md"

lint-shell: ## Run shellcheck on shell scripts
	find . -name '*.sh' -not -path './.git/*' -print0 | xargs -r -0 shellcheck

lint: lint-go lint-md lint-shell ## Run all linters (alias)
	go mod tidy

scan: ## Run security vulnerability scan
	@echo "TODO: CodeQL scanning runs in CI via GitHub Actions shared-codeql workflow"
	@echo "TODO: add local scanning tool here?"

# Release
generate: ## run general all file generation
	@echo "TODO: add proper generation here"

generate-version: ## Bump version in VERSION.yaml
	@hack/update_version.sh

generate-changelog: ## Update the local CHANGELOG.md file
	@hack/update_changelog.sh && $(MAKE) lint-md

get-version: ## Prints out the current version as presented in the VERSION.yaml file
	@echo $(VERSION)

test-changelog: ## Run the local changelog generation test suite
	@echo "$(green) testing changelog generation$(reset)"
	@hack/update_changelog_test.sh

docker-build: ## Build the container image for rootstock
	@echo "$(green) building docker image$(reset)"
	docker build \
		--build-arg GOCACHE=$(shell go env GOCACHE) \
		--build-arg GOMODCACHE=$(shell go env GOMODCACHE) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_TREE_STATE=$(GIT_TREE_STATE) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--platform $(PLATFORM) \
		-t ${IMAGE} \
		-f Dockerfile \
		.

# ---------------------------------------------------------------------------
# Local Kubernetes deployment via kind + Helm
# Requires: docker, kind, kubectl, helm
#
# All kind lifecycle logic lives in hack/kind/control.sh; the targets below are
# thin wrappers that pass the Makefile variables to it as environment overrides.
# One-shot: make kind-up (create cluster, load image, deploy mockup + chart)
#           make kind-verify (port-forward and probe the running pod)
#           make kind-down (delete the cluster)
# ---------------------------------------------------------------------------

# Invocation shared by every kind-* target: export the Makefile variables as the
# environment overrides that control.sh reads (RELEASE_NAME/RELEASE_NAMESPACE
# mirror CHART_RELEASE/CHART_NAMESPACE).
KIND_CONTROL := KIND_CLUSTER=$(KIND_CLUSTER) \
	RELEASE_NAME=$(CHART_RELEASE) \
	RELEASE_NAMESPACE=$(CHART_NAMESPACE) \
	REDFISH_SECRET=$(REDFISH_SECRET) \
	REDFISH_PASSWORD=$(REDFISH_PASSWORD) \
	IMAGE=$(IMAGE) \
	hack/kind/control.sh

helm-lint: ## Lint the Helm chart
	@echo "$(green) helm lint$(reset)"
	helm lint $(CHART_DIR) -f $(CHART_DIR)/ci/test-values.yaml

helm-template: ## Render the Helm chart to stdout
	@helm template $(CHART_RELEASE) $(CHART_DIR) -f $(CHART_DIR)/ci/test-values.yaml

kind-cluster: ## Create the kind cluster if it does not exist
	@$(KIND_CONTROL) create $(KIND_CLUSTER)

kind-prepare: kind-cluster ## Build/load the mockup image, create namespace and credentials secret
	@$(KIND_CONTROL) prepare

kind-deploy: docker-build kind-cluster ## Build/load the rootstock image and install/upgrade the Helm chart
	@$(KIND_CONTROL) deploy

kind-up: kind-prepare kind-deploy ## Create kind cluster and deploy everything
	@echo "$(green) rootstock is deployed. Run 'make kind-verify' to probe it.$(reset)"

kind-verify: ## Port-forward and probe the running rootstock pod
	@$(KIND_CONTROL) validate

kind-cleanup: ## Uninstall rootstock and delete all cluster resources
	@$(KIND_CONTROL) cleanup

kind-down: ## Delete the kind cluster
	@$(KIND_CONTROL) delete $(KIND_CLUSTER)

# Build everything that is necessary for testing prometheus locally
prometheus-local-setup: build mockup-start prometheus-collector-start

# Stop and remove docker images for the prometheus local setup
prometheus-local-cleanup: mockup-stop prometheus-collector-stop

prometheus-collector-start: ## Start Prometheus in Docker to scrape rootstock metrics
	docker run -d --name prometheus -p 9090:9090 -v $(CURDIR)/prometheus.yaml:/etc/prometheus/prometheus.yml prom/prometheus:latest

prometheus-collector-stop: ## Stop and remove Prometheus container
	docker stop prometheus || true
	docker rm prometheus || true


# Run the go binary with the redfish config necessary for prometheus local test
run-redfish: build ## Build and run the rootstock binary with redfish credentials for prometheus testing
	@mkdir -p tmp
	@echo "admin" > tmp/redfish_password
	./bin/rootstock run \
		--config-folder tmp \
		--redfish-credentials-file redfish_password

# Setup Prometheus locally and run the binary 
prometheus-local-run: prometheus-local-setup run-redfish

# Help

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'
