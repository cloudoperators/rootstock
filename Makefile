CURRENT_DIR=$(shell pwd)

green=\033[1;32m[+]
reset=\033[0m

OS              =$(shell go env GOOS)
ARCH            =$(shell go env GOARCH)
VERSION					=$(shell cat ${CURRENT_DIR}/VERSION.yaml | yq -r '.version')
GIT_COMMIT      =$(shell git rev-parse HEAD)
GIT_TREE_STATE  =$(shell if [ -z "`git status --porcelain`" ]; then echo "clean" ; else echo "dirty"; fi)
BUILD_DATE      =$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

override LDFLAGS += \
	-X main.version=$(VERSION) \
	-X main.buildDate=$(BUILD_DATE) \
	-X main.gitCommit=$(GIT_COMMIT) \
	-X main.gitTreeState=$(GIT_TREE_STATE)


.PHONY: test-unit
test-unit: ## Run unit tests
	@echo "$(green) unit testing$(reset)"
  test_dir="$(shell go list ./... | grep -v \
		-e tmp \
		-e pkg/generated \
	)"; \
	CGO_ENABLED=1 go test $$test_dir -v -tags=unit \
		-coverprofile \
		-covermode=count \
		-coverprofile=coverage.out;
	go tool cover -func=coverage.out -o=coverage.out

.PHONY: bin
bin: ## Creates the binary in the operating systems architecture
	@echo "$(green) building binary$(reset)"
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) GO111MODULE=on \
		go build \
			-installsuffix 'static' \
			-a \
			-o "bin/rootstock" \
			-ldflags "${LDFLAGS}" \
			cmd/server/main.go


.PHONY: help
help:
	@printf "\033[36m%-30s\033[0m %s\n" "% placeholder options" $(COMPONENTS)
	@grep -E '^[%a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
