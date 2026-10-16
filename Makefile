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

IMAGE ?= ghcr.io/cloudoperators/rootstock:dev-$(USER)

# Integration test mockup server
# Requires pdu-mockup data at testdata/pdu-mockup (already added to the repo).
# Or override the path: make test-integration PDU_MOCKUP_PATH=/your/path/to/pdu-mockup

PDU_MOCKUP_PATH  ?= $(CURDIR)/testdata/pdu-mockup
MOCKUP_PORT      ?= 8000
MOCKUP_CONTAINER := redfish-mockup



.PHONY: build test test-unit test-integration test-contract test-performance \
				mockup-start mockup-stop \
				lint lint-go lint-md lint-shell \
				scan \
				docker-build \
				generate generate-changelog generate-version \
				get-version test-changelog \
				help

# Build

build: GIT_COMMIT  = $(shell git rev-parse HEAD)
build: GIT_STATE   = $(shell if [ -z "$$(git status --porcelain)" ]; then echo clean; else echo dirty; fi)
build: BUILD_DATE  = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
build: LDFLAGS     = "-s -w \
											-X main.version=$(VERSION) \
											-X main.gitCommit=$(GIT_COMMIT) \
											-X main.gitTreeState=$(GIT_STATE) \
											-X main.buildDate=$(BUILD_DATE)"
build: ## Compile a static server binary to bin/
	@echo "$(green) building binary$(reset)"
	CGO_ENABLED=0 go build \
			-installsuffix 'static' \
			-ldflags $(LDFLAGS) \
			-o bin/rootstock \
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
		--platform $(PLATFORM) \
		-t ${IMAGE} \
		-f Dockerfile \
		.

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
