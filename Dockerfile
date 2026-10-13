# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0

##########################################################################################
# Builder image
##########################################################################################
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.27.1 AS builder

ARG GOCACHE="/root/.cache/go-build"
ARG GOMODCACHE="/go/pkg/mod"
ARG TARGETOS
ARG TARGETARCH

ARG VERSION
ARG BUILD_DATE
ARG GIT_COMMIT
ARG GIT_TREE_STATE

WORKDIR /src


COPY . .

RUN --mount=type=cache,target=${GOMODCACHE} \
      go mod download

RUN --mount=type=cache,target=${GOMODCACHE} \
    --mount=type=cache,target=${GOCACHE} \
      make build \
        GOFIPS140=certified \
        CGO_ENABLED=0 \
        GOOS=${TARGETOS} \
        GOARCH=${TARGETARCH} \
        VERSION=${VERSION} \
        GIT_COMMIT=${GIT_COMMIT} \
        GIT_TREE_STATE=${GIT_TREE_STATE} \
        BUILD_DATE=${BUILD_DATE}


##########################################################################################
# Final image
##########################################################################################
FROM gcr.io/distroless/static:nonroot

LABEL source_repository="https://github.com/cloudoperators/rootstock"
WORKDIR /
COPY --from=builder /src/bin/rootstock rootstock
USER 65531:65531


# FIPS mode: always enforce strict FIPS 140 compliance at runtime
ENV GODEBUG=fips140=only

ENTRYPOINT ["/rootstock"]
