# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0


BINARY         := rootstock
OS             := $(shell go env GOOS)
ARCH           := $(shell go env GOARCH)
VERSION        := $(shell command -v yq > /dev/null 2>&1 \
	&& yq -r '.version' VERSION.yaml \
	|| git describe --tags --always --dirty 2>/dev/null \
	|| echo "dev")
GIT_COMMIT     := $(shell git rev-parse HEAD)
GIT_TREE_STATE := $(shell if [ -z "`git status --porcelain`" ]; then echo "clean"; else echo "dirty"; fi)
BUILD_DATE     := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

override LDFLAGS += \
	-X main.version=$(VERSION) \
	-X main.buildDate=$(BUILD_DATE) \
	-X main.gitCommit=$(GIT_COMMIT) \
	-X main.gitTreeState=$(GIT_TREE_STATE)


# Integration test mockup server
# Requires pdu-mockup data at testdata/pdu-mockup (already added to the repo).
# Or override the path: make test-integration PDU_MOCKUP_PATH=/your/path/to/pdu-mockup

PDU_MOCKUP_PATH  ?= $(CURDIR)/testdata/pdu-mockup
MOCKUP_PORT      ?= 8000
MOCKUP_CONTAINER := redfish-mockup

green := \033[1;32m[+]
reset := \033[0m

.PHONY: build test test-integration test-contract test-performance \
        mockup-start mockup-stop \
        lint conventions scan \
        generate docker-build \
        help

# Build

build: ## Compile a static server binary to bin/
	@echo "$(green) building binary$(reset)"
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) \
		go build \
			-installsuffix 'static' \
			-a \
			-o bin/$(BINARY) \
			-ldflags "$(LDFLAGS)" \
			.

# Tests

test: ## Run unit tests with coverage report
	@echo "$(green) unit testing$(reset)"
	CGO_ENABLED=1 go test ./... -parallel=10 -timeout=10m \
		-covermode=count \
		-coverprofile=coverage.out
	go tool cover -func=coverage.out -o=coverage.out

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
lint: ## Run golangci-lint
	golangci-lint run ./...

conventions: ## Check formatting conventions - markdown, shell, go (TODO: prettier + shellcheck)
	@echo "TODO: run prettier on markdown files"
	@echo "TODO: run shellcheck on shell scripts"
	$(MAKE) lint

scan: ## Run security vulnerability scan
	@echo "TODO: CodeQL scanning runs in CI via GitHub Actions shared-codeql workflow"
	@echo "TODO: add local scanning tool here?"

# Release
generate: ## Run code generation (TODO: not yet needed)
	@echo "TODO: add code generation steps e.g. go generate ./..."

docker-build: ## Build the container image (TODO: Dockerfile not yet written)
	@echo "TODO: docker build -t $(BINARY):$(VERSION) ."

# Help

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'
