#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../_helper.sh
source "${SCRIPT_DIR}/../_helper.sh"

# Usage: verify.sh <kind-cluster> <namespace> <release>
KIND_CLUSTER="${1:-rootstock-local}"
NAMESPACE="${2:-rootstock}"
RELEASE="${3:-rootstock}"
CONTEXT="kind-${KIND_CLUSTER}"
LOCAL_PORT="${LOCAL_PORT:-9220}"
# The in-cluster Redfish mockup service (deployed by `make kind-mockup`).
TARGET="${TARGET:-http://redfish-mockup.${NAMESPACE}.svc.cluster.local:8000}"

log_info "Waiting for the rootstock deployment to be ready..."
_exec kubectl --context "${CONTEXT}" -n "${NAMESPACE}" rollout status \
    "deploy/${RELEASE}" --timeout=120s

SVC="${RELEASE}"
PF_PID=""
cleanup() {
    # shellcheck disable=SC2015
    [[ -n "${PF_PID}" ]] && kill "${PF_PID}" 2>/dev/null || true
}
trap cleanup EXIT

log_info "Port-forwarding svc/${SVC} ${LOCAL_PORT}:9220 ..."
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" \
    port-forward "svc/${SVC}" "${LOCAL_PORT}:9220" >/dev/null 2>&1 &
PF_PID=$!

# Wait for the port-forward / server to answer /healthz.
log_info "Waiting for /healthz ..."
for _ in $(seq 1 30); do
    if curl -sf "http://localhost:${LOCAL_PORT}/healthz" >/dev/null 2>&1; then
        log_info "  /healthz OK"
        break
    fi
    sleep 1
done

curl -sf "http://localhost:${LOCAL_PORT}/healthz" >/dev/null \
    || die "/healthz never became ready"

log_info "Scraping /probe against ${TARGET} ..."
PROBE_URL="http://localhost:${LOCAL_PORT}/probe?target=${TARGET}&labels=1"
BODY="$(curl -sf "${PROBE_URL}")" \
    || die "/probe request failed"

echo "----- /probe output (head) -----"
echo "${BODY}" | head -n 20
echo "--------------------------------"

# Assert at least one rootstock/pdu metric is present (the dataset exposes
# apparent power / power readings). Accept a few plausible metric name stems.
if echo "${BODY}" | grep -Eqi 'pdu_|power|energy|apparent'; then
    log_info "SUCCESS: rootstock scraped the Redfish mockup and emitted metrics."
else
    die "no expected metrics found in /probe output."
fi
