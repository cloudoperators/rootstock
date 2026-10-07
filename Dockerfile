# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0

# Build the rootstock binary.
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.26 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=99.99.99
ARG GIT_COMMIT=""
ARG BUILD_DATE="1970-01-01T00:00:00Z"
ARG GIT_TREE_STATE="clean"

ENV CGO_ENABLED=0

WORKDIR /workspace

# Cache dependencies first.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy the rest of the sources and build a static binary.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
      -installsuffix 'static' \
      -a \
      -o /workspace/bin/rootstock \
      -ldflags "-X main.version=${VERSION} -X main.buildDate=${BUILD_DATE} -X main.gitCommit=${GIT_COMMIT} -X main.gitTreeState=${GIT_TREE_STATE}" \
      .

# Use distroless as a minimal, non-root base image.
# Refer to https://github.com/GoogleContainerTools/distroless for more details.
FROM --platform=${BUILDPLATFORM:-linux/amd64} gcr.io/distroless/static:nonroot
LABEL source_repository="https://github.com/cloudoperators/rootstock"
WORKDIR /
COPY --from=builder /workspace/bin/rootstock /rootstock
USER 65532:65532

ENTRYPOINT ["/rootstock"]
