#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Defaults — mirror the Makefile variables
KIND_CLUSTER="${KIND_CLUSTER:-rootstock-local}"
CHART_DIR="${REPO_ROOT}/k8s_configuration/chart"
RELEASE_NAME="${RELEASE_NAME:-rootstock}"
RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-rootstock}"
REDFISH_SECRET="${REDFISH_SECRET:-rootstock-redfish}"
REDFISH_PASSWORD="${REDFISH_PASSWORD:-admin}"
IMAGE="${IMAGE:-ghcr.io/cloudoperators/rootstock:dev-${USER}}"

# Split IMAGE into repo and tag (colon in the last path segment signals a tag)
_image_last="${IMAGE##*/}"
if [[ "${_image_last}" == *:* ]]; then
    IMAGE_TAG="${IMAGE##*:}"
    IMAGE_REPO="${IMAGE%:"${IMAGE_TAG}"}"
else
    IMAGE_TAG="latest"
    IMAGE_REPO="${IMAGE}"
fi

CONTEXT="kind-${KIND_CLUSTER}"

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    CHART_NAMESPACE="$(_resolve_chart_namespace)"
    CHART_RELEASE="$(_resolve_chart_release)"

    local command="${1:-}"
    shift || true

    case "${command}" in
        create)   cmd_create "$@" ;;
        delete)   cmd_delete "$@" ;;
        prepare)  cmd_prepare ;;
        cleanup)  cmd_cleanup ;;
        deploy)   cmd_deploy ;;
        validate) cmd_validate ;;
        help|--help|-h) usage ;;
        "")
            err "No command specified."
            echo >&2
            usage >&2
            exit 1
            ;;
        *)
            err "Unknown command: '${command}'"
            echo >&2
            usage >&2
            exit 1
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() { echo -e "\033[1;32m[+]\033[0m $*"; }
err() { echo -e "\033[1;31m[!]\033[0m $*" >&2; }

_resolve_chart_namespace() {
    # Render the servicemonitor template (which embeds rootstock.namespace) and
    # extract the metadata.namespace field. This mirrors the Helm helper:
    #   {{- default .Release.Namespace .Values.namespace }}
    # Falls back to RELEASE_NAMESPACE if helm is unavailable or rendering fails.
    local ns
    ns="$(helm template "${CHART_DIR}" \
        --set serviceMonitor.enabled=true \
        --api-versions monitoring.coreos.com/v1/ServiceMonitor \
        --show-only templates/servicemonitor.yaml \
        2>/dev/null \
        | awk '/^  namespace:/{print $2; exit}')"
    echo "${ns}"
}

_resolve_chart_release() {
    # Render the serviceaccount template and extract app.kubernetes.io/instance,
    # which reflects the rootstock.releaseName helper:
    #   {{- default .Release.Name .Values.releaseName }}
    # Falls back to RELEASE_NAME if helm is unavailable or rendering fails.
    local release
    release="$(helm template "${RELEASE_NAME}" "${CHART_DIR}" \
        --namespace "${RELEASE_NAMESPACE}" \
        --set redfish.existingSecret=placeholder \
        --show-only templates/serviceaccount.yaml \
        2>/dev/null \
        | awk '/app\.kubernetes\.io\/instance:/{print $2; exit}')"
    echo "${release:-${RELEASE_NAME}}"
}

usage() {
    cat <<EOF
Usage: $(basename "${BASH_SOURCE[0]}") <command> [args]

Commands:
  create   [NAME]   Create a kind cluster (default: ${KIND_CLUSTER})
  delete   [NAME]   Delete a kind cluster (default: ${KIND_CLUSTER})
  prepare           Build/load mockup image, create namespace and credentials secret
  cleanup           Uninstall rootstock and delete all cluster resources
  deploy            Build/load rootstock image and install/upgrade the Helm chart
  validate          Validate the running rootstock deployment via port-forward + probe

Environment variables (override Makefile defaults):
  KIND_CLUSTER      Cluster name          (default: rootstock-local)
  RELEASE_NAME      Helm release name     (default: rootstock)
  RELEASE_NAMESPACE Kubernetes namespace  (default: rootstock)
  REDFISH_SECRET    Credentials secret    (default: rootstock-redfish)
  REDFISH_PASSWORD  Redfish password      (default: admin)
  IMAGE             Container image       (default: ghcr.io/cloudoperators/rootstock:dev-\$USER)
EOF
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

cmd_create() {
    local name="${1:-${KIND_CLUSTER}}"
    if kind get clusters 2>/dev/null | grep -qx "${name}"; then
        log "Kind cluster '${name}' already exists — skipping."
        return
    fi
    log "Creating kind cluster '${name}'..."
    kind create cluster --name "${name}"
    log "Kind cluster '${name}' is ready."
}

cmd_delete() {
    local name="${1:-${KIND_CLUSTER}}"
    log "Deleting kind cluster '${name}'..."
    kind delete cluster --name "${name}"
}

cmd_prepare() {
    log "Building redfish mockup image..."
    docker build -t rootstock-mockup:latest \
        -f "${SCRIPT_DIR}/Dockerfile.mockup" "${REPO_ROOT}"

    log "Loading mockup image into kind cluster '${KIND_CLUSTER}'..."
    kind load docker-image rootstock-mockup:latest --name "${KIND_CLUSTER}"

    log "Ensuring namespace '${CHART_NAMESPACE}'..."
    kubectl --context "${CONTEXT}" create namespace "${CHART_NAMESPACE}" \
        --dry-run=client -o yaml | kubectl --context "${CONTEXT}" apply -f -

    log "Deploying redfish mockup..."
    kubectl --context "${CONTEXT}" -n "${CHART_NAMESPACE}" \
        apply -f "${SCRIPT_DIR}/redfish-mockup.yaml"
    kubectl --context "${CONTEXT}" -n "${CHART_NAMESPACE}" \
        rollout status deploy/redfish-mockup --timeout=120s

    log "Creating redfish credentials secret '${REDFISH_SECRET}'..."
    kubectl --context "${CONTEXT}" -n "${CHART_NAMESPACE}" \
        create secret generic "${REDFISH_SECRET}" \
        --from-literal=redfish_password="${REDFISH_PASSWORD}" \
        --dry-run=client -o yaml | kubectl --context "${CONTEXT}" apply -f -

    log "Cluster prepared — run 'deploy' to install rootstock."
}

cmd_cleanup() {
    log "Uninstalling Helm release '${RELEASE_NAME}'..."
    helm --kube-context "${CONTEXT}" uninstall "${RELEASE_NAME}" \
        --namespace "${CHART_NAMESPACE}" 2>/dev/null || true

    log "Deleting namespace '${CHART_NAMESPACE}'..."
    kubectl --context "${CONTEXT}" delete namespace "${CHART_NAMESPACE}" \
        --ignore-not-found --wait

    log "Cleanup complete."
}

cmd_deploy() {
    local version
    version="$(cd "${REPO_ROOT}" && \
        command -v yq > /dev/null 2>&1 \
            && yq -r '.version' VERSION.yaml \
            || echo "0.0.0-$(git rev-parse --short HEAD)")"

    log "Building container image '${IMAGE}'..."
    docker build \
        --build-arg VERSION="${version}" \
        --build-arg GIT_COMMIT="$(cd "${REPO_ROOT}" && git rev-parse HEAD 2>/dev/null || echo '')" \
        --build-arg GIT_TREE_STATE="$(cd "${REPO_ROOT}" && \
            [ -z "$(git status --porcelain 2>/dev/null)" ] && echo clean || echo dirty)" \
        --build-arg BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
        -t "${IMAGE}" \
        -f "${REPO_ROOT}/Dockerfile" \
        "${REPO_ROOT}"

    log "Loading '${IMAGE}' into kind cluster '${KIND_CLUSTER}'..."
    kind load docker-image "${IMAGE}" --name "${KIND_CLUSTER}"

    log "Installing/upgrading Helm chart '${RELEASE_NAME}'..."
    helm --kube-context "${CONTEXT}" upgrade --install "${RELEASE_NAME}" "${CHART_DIR}" \
        --namespace "${CHART_NAMESPACE}" --create-namespace \
        --set image.repository="${IMAGE_REPO}" \
        --set image.tag="${IMAGE_TAG}" \
        --set image.pullPolicy=IfNotPresent \
        --set redfish.existingSecret="${REDFISH_SECRET}" \
        --wait --timeout 120s

    log "Rootstock deployed — run 'validate' to probe it."
}

cmd_validate() {
    "${SCRIPT_DIR}/verify.sh" "${KIND_CLUSTER}" "${CHART_NAMESPACE}" "${CHART_RELEASE}"
}

# ---------------------------------------------------------------------------

main "$@"
