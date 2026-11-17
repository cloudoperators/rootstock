#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# shellcheck source=../_helper.sh
source "${REPO_ROOT}/hack/_helper.sh"

# Defaults — mirror the Makefile variables
KIND_CLUSTER="${KIND_CLUSTER:-rootstock}"
CHART_DIR="${REPO_ROOT}/k8s_configuration/chart"
# Lowercase ${USER}: the dev tag doubles as a Helm chart version, from which
# Greenhouse derives RFC 1123 (lowercase-only) Kubernetes object names.
USER_LC="$(printf '%s' "${USER}" | tr '[:upper:]' '[:lower:]')"
IMAGE="${IMAGE:-ghcr.io/cloudoperators/rootstock:0.0.0-dev-${USER_LC}}"
# Optional additional Helm values file applied during deploy. Skipped if missing.
VALUES_FILE="${VALUES_FILE:-${REPO_ROOT}/tmp/local_values.yaml}"

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
    local deployment

    _exec_output deployment helm template "${CHART_DIR}" --show-only templates/serviceaccount.yaml
    _exec_output CHART_NAMESPACE "echo '${deployment}' | yq -r '.metadata.namespace'"
    _exec_output CHART_RELEASE "echo '${deployment}' | yq -r '.metadata.labels.\"app.kubernetes.io/instance\"'"

    local command="${1:-}"
    shift || true

    case "${command}" in
        create)   cmd_create "$@" ;;
        delete)   cmd_delete "$@" ;;
        prepare)  cmd_prepare ;;
        cleanup)  cmd_cleanup ;;
        deploy)   cmd_deploy "$@" ;;
        validate) cmd_validate ;;
        help|--help|-h) usage ;;
        "")
            log_error "No command specified."
            echo >&2
            usage >&2
            exit 1
            ;;
        *)
            log_error "Unknown command: '${command}'"
            echo >&2
            usage >&2
            exit 1
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

usage() {
    cat <<EOF
Usage: $(basename "${BASH_SOURCE[0]}") <command> [args]

Commands:
  create   [NAME]   Create a kind cluster (default: ${KIND_CLUSTER})
  delete   [NAME]   Delete a kind cluster (default: ${KIND_CLUSTER})
  prepare           Build/load mockup image, create namespace and credentials secret
  cleanup           Uninstall rootstock and delete all cluster resources
  deploy            Build/load rootstock image and install/upgrade the Helm chart
                    [--values FILE]  Additional Helm values file
                                     (default: ./tmp/local_values.yaml; skipped if missing)
  validate          Validate the running rootstock deployment via port-forward + probe

Environment variables (override Makefile defaults):
  KIND_CLUSTER         Cluster name                          (default: rootstock-local)
  IMAGE                Container image
                       (default: ghcr.io/cloudoperators/rootstock:0.0.0-dev-\$USER)
  VALUES_FILE          Additional Helm values file for 'deploy'
                       (default: ./tmp/local_values.yaml; skipped if missing)
EOF
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

cmd_create() {
    local name="${1:-${KIND_CLUSTER}}"
    if kind get clusters 2>/dev/null | grep -qx "${name}"; then
        log_info "Kind cluster '${name}' already exists — skipping."
        return
    fi
    log_info "Creating kind cluster '${name}'..."
    _exec kind create cluster --name "${name}"
    log_info "Kind cluster '${name}' is ready."
}

cmd_delete() {
    local name="${1:-${KIND_CLUSTER}}"
    log_info "Deleting kind cluster '${name}'..."
    _exec kind delete cluster --name "${name}"
}

cmd_prepare() {
    log_info "Building redfish mockup image..."
    _exec docker build -t rootstock-mockup:latest \
        -f "${SCRIPT_DIR}/Dockerfile.mockup" "${REPO_ROOT}"

    log_info "Loading mockup image into kind cluster '${KIND_CLUSTER}'..."
    _exec kind load docker-image rootstock-mockup:latest --name "${KIND_CLUSTER}"

    log_info "Ensuring namespace '${CHART_NAMESPACE}'..."
    _exec "kubectl --context '${CONTEXT}' create namespace '${CHART_NAMESPACE}' \
        --dry-run=client -o yaml | kubectl --context '${CONTEXT}' apply -f -"

    log_info "Deploying redfish mockup..."
    _exec kubectl --context "${CONTEXT}" -n "${CHART_NAMESPACE}" \
        apply -f "${SCRIPT_DIR}/redfish-mockup.yaml"
    _exec kubectl --context "${CONTEXT}" -n "${CHART_NAMESPACE}" \
        rollout status deploy/redfish-mockup --timeout=120s

    log_info "Cluster prepared — run 'deploy' to install rootstock."
}

cmd_cleanup() {
    log_info "Uninstalling Helm release '${CHART_RELEASE}'..."
    _exec helm --kube-context "${CONTEXT}" uninstall "${CHART_RELEASE}" \
        --namespace "${CHART_NAMESPACE}" 2>/dev/null || true

    log_info "Deleting namespace '${CHART_NAMESPACE}'..."
    _exec kubectl --context "${CONTEXT}" delete namespace "${CHART_NAMESPACE}" \
        --ignore-not-found --wait

    log_info "Cleanup complete."
}

cmd_deploy() {
    local values_file="${VALUES_FILE}"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --values) values_file="$2"; shift 2 ;;
            *) log_error "Unknown deploy option: '$1'"; exit 1 ;;
        esac
    done

    log_info "Loading '${IMAGE}' into kind cluster '${KIND_CLUSTER}'..."
    _exec kind load docker-image "${IMAGE}" --name "${KIND_CLUSTER}"

    log_info "Installing/upgrading Helm chart '${CHART_RELEASE}'..."

    local values_args=()
    if [[ -f "${values_file}" ]]; then
        log_info "Using additional values file '${values_file}'."
        values_args=(--values "${values_file}")
    else
        log_info "Values file '${values_file}' not found — skipping --values."
    fi

    _exec helm --kube-context "${CONTEXT}" upgrade --install "${CHART_RELEASE}" "${CHART_DIR}" \
        --namespace "${CHART_NAMESPACE}" --create-namespace \
        --set image.repository="${IMAGE_REPO}" \
        --set image.tag="${IMAGE_TAG}" \
        --set image.pullPolicy=IfNotPresent \
        "${values_args[@]}" \
        --wait --timeout 30s

    log_info "Rootstock deployed — run 'validate' to probe it."
}

cmd_validate() {
    _exec "${SCRIPT_DIR}/verify.sh" "${KIND_CLUSTER}" "${CHART_NAMESPACE}" "${CHART_RELEASE}"
}

# ---------------------------------------------------------------------------

main "$@"
