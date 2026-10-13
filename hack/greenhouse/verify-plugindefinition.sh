#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# verify-plugindefinition.sh
# -----------------------------------------------------------------------------
# Self-contained verifier for the rootstock Greenhouse PluginDefinition.
#
# It bootstraps a local Greenhouse-in-kind from scratch (no pre-existing
# Greenhouse checkout required), publishes the rootstock Helm chart to the
# Greenhouse-provided local OCI registry, applies the PluginDefinition, and
# asserts that it reconciles to READY=True.
#
# Default run (fast path):
#   1. preflight tool checks
#   2. git clone cloudoperators/greenhouse at a pinned ref
#   3. `make setup` (brings up admin+remote kind clusters, registry, demo org)
#   4. helm package + push the rootstock chart to the local registry
#   5. apply the PluginDefinition (temp copy, repository rewritten) and
#      assert READY=True   <-- primary pass/fail signal
#
# Opt-in full end-to-end (--with-plugin):
#   6. load the rootstock image into the remote cluster, create the redfish
#      Secret + mockup, apply a Plugin, assert it is Ready, then port-forward
#      and assert /healthz + /probe emit real metrics.
#
# Teardown runs by default; pass --keep to retain clusters + clone for debugging.
#
# Everything is overridable via environment variables (see below). The committed
# plugindefinition.yaml is NEVER mutated -- all rewrites happen on temp copies.
# -----------------------------------------------------------------------------
set -euo pipefail

# --- Configuration (all overridable) -----------------------------------------
GREENHOUSE_REPO="${GREENHOUSE_REPO:-https://github.com/cloudoperators/greenhouse}"
GREENHOUSE_REF="${GREENHOUSE_REF:-v0.17.2}"

# Cluster / org / context names (match the pinned Greenhouse Makefile defaults).
ADMIN_CLUSTER="${ADMIN_CLUSTER:-greenhouse-admin}"
REMOTE_CLUSTER="${REMOTE_CLUSTER:-greenhouse-remote}"
ADMIN_CONTEXT="kind-${ADMIN_CLUSTER}"
REMOTE_CONTEXT="kind-${REMOTE_CLUSTER}"
DEMO_ORG="${DEMO_ORG:-demo}"

# Local registry provided by Greenhouse's hack/registry/setup.sh (zot).
#   - REGISTRY_HOST  : how the host (helm push) reaches it.
#   - REGISTRY_INCLUSTER : how kind nodes / Flux reach it (docker "kind" network).
REGISTRY_HOST="${REGISTRY_HOST:-localhost:5000}"
REGISTRY_INCLUSTER="${REGISTRY_INCLUSTER:-registry:5000}"
# OCI path (without the oci:// scheme) under which the chart is pushed.
REGISTRY_CHART_REPO="${REGISTRY_CHART_REPO:-charts}"

# rootstock image coordinates (built by `make build` / `make kind-up`).
# Default tag is resolved dynamically from VERSION.yaml in resolve_versions()
# (matching the Makefile's single-source-of-truth). Set ROOTSTOCK_IMAGE to pin.
ROOTSTOCK_IMAGE="${ROOTSTOCK_IMAGE:-}"

# Chart version packaged, pushed and requested by the PluginDefinition.
#   - Authority: k8s_configuration/chart/Chart.yaml `version`.
#   - Override via CHART_VERSION to test a different version (e.g. an old tag).
#     NOTE: this repackages the *current* chart source under that version number;
#     it does not check out the historical chart templates.
CHART_VERSION="${CHART_VERSION:-}"
# Application/CLI version (image tag source). Authority: VERSION.yaml `version`.
APP_VERSION="${APP_VERSION:-}"

# Polling.
READY_TIMEOUT="${READY_TIMEOUT:-300}"   # seconds to wait for PluginDefinition/Plugin READY
POLL_INTERVAL="${POLL_INTERVAL:-5}"

# Flags (set by argument parsing).
WITH_PLUGIN=false
KEEP=false

# --- Paths --------------------------------------------------------------------
ROOTSTOCK_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKDIR="${WORKDIR:-${ROOTSTOCK_ROOT}/.greenhouse-verify}"
GREENHOUSE_DIR="${WORKDIR}/greenhouse"
CHART_DIR="${ROOTSTOCK_ROOT}/k8s_configuration/chart"
PLUGINDEF_SRC="${ROOTSTOCK_ROOT}/k8s_configuration/plugindefinition.yaml"
GH_TEST_DIR="${ROOTSTOCK_ROOT}/k8s_configuration/greenhouse-test"
TMPDIR_RUN=""

# --- Logging helpers ----------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  OK\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  !!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
    cat <<EOF
Usage: $(basename "$0") [--with-plugin] [--keep] [-h|--help]

  --with-plugin   Also run the full E2E: load image into the remote cluster,
                  create the Redfish Secret + mockup, apply a Plugin, assert it
                  is Ready, and verify /healthz + /probe emit real metrics.
  --keep          Do not tear down kind clusters / clone on exit (for debugging).
  -h, --help      Show this help.

Key environment overrides:
  GREENHOUSE_REF  (default: ${GREENHOUSE_REF})   Greenhouse git tag/commit to pin.
  WORKDIR         (default: <repo>/.greenhouse-verify)
  READY_TIMEOUT   (default: ${READY_TIMEOUT}s)
  CHART_VERSION   (default: k8s_configuration/chart/Chart.yaml .version)
                  Chart version to package/push/test. Override to test another
                  version (repackaged from the current chart source).
  APP_VERSION     (default: VERSION.yaml .version)  App/image tag version.
  ROOTSTOCK_IMAGE (default: rootstock:<APP_VERSION>) Image used by --with-plugin.
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --with-plugin) WITH_PLUGIN=true ;;
            --keep)        KEEP=true ;;
            -h|--help)     usage; exit 0 ;;
            *)             usage; die "unknown argument: $1" ;;
        esac
        shift
    done
}

# =============================================================================
# Phase 0: preflight
# =============================================================================
preflight() {
    log "Phase 0: preflight tool checks"
    local missing=()
    local t
    for t in docker kind kubectl helm yq go git jq; do
        command -v "$t" >/dev/null 2>&1 || missing+=("$t")
    done
    if ((${#missing[@]})); then
        die "missing required tools: ${missing[*]}"
    fi
    docker info >/dev/null 2>&1 || die "docker daemon is not reachable (is Docker running?)"
    [[ -d "$CHART_DIR" ]] || die "rootstock chart not found at ${CHART_DIR}"
    [[ -f "$PLUGINDEF_SRC" ]] || die "plugindefinition.yaml not found at ${PLUGINDEF_SRC}"

    # Greenhouse's hack/registry/setup.sh hard-codes the local registry on host
    # port 5000 (not overridable). Detect a conflict NOW -- before spending minutes
    # creating kind clusters only to fail inside `make setup`.
    check_registry_port
    ok "all tools present and docker daemon reachable"
}

# Ensure host port 5000 is free for Greenhouse's local registry (zot). A stale
# `registry` container from a previous run is cleaned up automatically; any other
# listener (notably macOS AirPlay Receiver / ControlCenter) must be freed by the
# user, because the Greenhouse setup script hard-codes port 5000.
check_registry_port() {
    local port=5000

    # A leftover `registry` container is ours to remove.
    if docker inspect registry >/dev/null 2>&1; then
        warn "removing stale 'registry' container from a previous run"
        docker rm -f registry >/dev/null 2>&1 || true
    fi

    local holder=""
    if command -v lsof >/dev/null 2>&1; then
        # `|| true`: lsof exits non-zero when nothing is listening, which would
        # otherwise abort the script under `set -e`/`pipefail`.
        holder="$(lsof -nP -iTCP:${port} -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $1; exit}' || true)"
    fi
    if [[ -n "$holder" ]]; then
        local hint="free the process using host port ${port} and re-run"
        if [[ "$holder" == ControlCe* ]]; then
            hint="macOS AirPlay Receiver is using port ${port}. Disable it in System Settings -> General -> AirDrop & Handoff -> AirPlay Receiver, then re-run"
        fi
        die "host port ${port} is in use by '${holder}' (needed by Greenhouse's local registry). ${hint}."
    fi
}


# =============================================================================
# Phase 1: fetch Greenhouse at the pinned ref
# =============================================================================
fetch_greenhouse() {
    log "Phase 1: fetch Greenhouse @ ${GREENHOUSE_REF}"
    mkdir -p "$WORKDIR"
    if [[ -d "${GREENHOUSE_DIR}/.git" ]]; then
        local have
        have="$(git -C "$GREENHOUSE_DIR" describe --tags --always 2>/dev/null || echo unknown)"
        if [[ "$have" == "$GREENHOUSE_REF" ]]; then
            ok "existing clone already at ${GREENHOUSE_REF}, reusing"
            return
        fi
        warn "existing clone is at '${have}', re-cloning at ${GREENHOUSE_REF}"
        rm -rf "$GREENHOUSE_DIR"
    fi
    git clone --depth 1 --branch "$GREENHOUSE_REF" "$GREENHOUSE_REPO" "$GREENHOUSE_DIR" \
        || die "git clone of ${GREENHOUSE_REPO}@${GREENHOUSE_REF} failed"
    ok "cloned Greenhouse into ${GREENHOUSE_DIR}"
}

# =============================================================================
# Phase 2: bring up Greenhouse (milestone stop)
# =============================================================================
bring_up_greenhouse() {
    log "Phase 2: bootstrap local Greenhouse (make setup) -- this is slow and can be flaky on first run"
    # `make setup` = setup-manager + setup-dashboard + setup-demo. It creates the
    # admin+remote kind clusters, installs cert-manager/flux/manager, stands up
    # the local zot registry (remote cluster postSetup -> `make zot`), applies the
    # demo Organization, and waits for it to be Ready.
    (
        cd "$GREENHOUSE_DIR"
        make setup
    ) || die "Greenhouse 'make setup' failed (see output above)"

    # Export admin kubeconfig context for subsequent kubectl calls. setup-demo
    # already switched the current context, but be explicit/robust.
    kubectl config get-contexts "$ADMIN_CONTEXT" >/dev/null 2>&1 \
        || die "expected kube-context ${ADMIN_CONTEXT} not found after setup"
    ok "Greenhouse is up (admin=${ADMIN_CONTEXT}, remote=${REMOTE_CONTEXT}, org ns=${DEMO_ORG})"

    # Sanity: the PluginDefinition CRD must exist, else apply will fail cryptically.
    kubectl --context "$ADMIN_CONTEXT" get crd plugindefinitions.greenhouse.sap >/dev/null 2>&1 \
        || die "PluginDefinition CRD not present on admin cluster -- setup incomplete"
    ok "PluginDefinition CRD present"
}

# =============================================================================
# Phase 3: publish the rootstock chart to the local registry
# =============================================================================
publish_chart() {
    log "Phase 3: package + push rootstock chart to oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO}"

    # Confirm the registry container exists (started by Greenhouse's make zot).
    if ! docker inspect registry >/dev/null 2>&1; then
        warn "docker container 'registry' not found; attempting Greenhouse 'make zot'"
        ( cd "$GREENHOUSE_DIR" && make zot ) || die "could not start local registry"
    fi

    local pkgdir="${TMPDIR_RUN}/chart"
    mkdir -p "$pkgdir"
    # --version overrides the chart version at package time without mutating
    # Chart.yaml, so CHART_VERSION (default: Chart.yaml .version) drives the
    # tarball name and OCI tag.
    helm package "$CHART_DIR" --version "$CHART_VERSION" -d "$pkgdir" >/dev/null \
        || die "helm package failed"
    local tgz="${pkgdir}/rootstock-${CHART_VERSION}.tgz"
    [[ -f "$tgz" ]] || die "expected packaged chart ${tgz} not found"
    ok "packaged $(basename "$tgz")"

    # Push over plain HTTP (zot is HTTP on localhost:5000).
    helm push "$tgz" "oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO}" --plain-http \
        || die "helm push to oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO} failed"
    ok "pushed chart to oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO}/rootstock:${CHART_VERSION}"
}

# =============================================================================
# Phase 4: apply PluginDefinition and assert READY=True  (primary signal)
# =============================================================================
wait_ready() {
    # wait_ready <kind> <name> <namespace-or-empty>
    local kind="$1" name="$2" ns="${3:-}"
    local nsargs=()
    [[ -n "$ns" ]] && nsargs=(-n "$ns")
    local deadline=$(( $(date +%s) + READY_TIMEOUT ))
    local status=""
    while (( $(date +%s) < deadline )); do
        status="$(kubectl --context "$ADMIN_CONTEXT" "${nsargs[@]}" get "$kind" "$name" \
            -o jsonpath='{.status.statusConditions.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)"
        if [[ "$status" == "True" ]]; then
            return 0
        fi
        printf '    waiting for %s/%s Ready (current: %s)\n' "$kind" "$name" "${status:-<none>}"
        sleep "$POLL_INTERVAL"
    done
    warn "timed out waiting for ${kind}/${name} Ready; last status: ${status:-<none>}"
    kubectl --context "$ADMIN_CONTEXT" "${nsargs[@]}" get "$kind" "$name" -o yaml 2>/dev/null \
        | yq '.status' - || true
    return 1
}

apply_plugindefinition() {
    log "Phase 4: apply PluginDefinition and assert READY=True"

    # Operate on a TEMP COPY -- never mutate the committed plugindefinition.yaml.
    # Rewrite helmChart.repository to the in-cluster registry address that Flux
    # (running inside the kind cluster) can resolve, and pin spec.version /
    # helmChart.version to the chart version that was just packaged and pushed
    # (CHART_VERSION) so Greenhouse pulls exactly that artifact.
    #
    # NOTE: PluginDefinition is a *namespaced* resource in Greenhouse. The
    # reconciler resolves the registry-mirror config from the Organization whose
    # name equals the object's namespace, so the object MUST live in the org
    # namespace (${DEMO_ORG}); applied cluster-scoped it defaults to namespace
    # "default" and never reconciles (org "default" not found). The global,
    # cluster-scoped catalog variant is a separate kind (ClusterPluginDefinition).
    local pd="${TMPDIR_RUN}/plugindefinition.yaml"
    cp "$PLUGINDEF_SRC" "$pd"
    yq -i "
      .metadata.namespace = \"${DEMO_ORG}\" |
      .spec.version = \"${CHART_VERSION}\" |
      .spec.helmChart.repository = \"oci://${REGISTRY_INCLUSTER}/${REGISTRY_CHART_REPO}\" |
      .spec.helmChart.version = \"${CHART_VERSION}\"
    " "$pd"
    ok "temp PluginDefinition: namespace=${DEMO_ORG}, version=${CHART_VERSION}, repository=oci://${REGISTRY_INCLUSTER}/${REGISTRY_CHART_REPO}"

    kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" apply -f "$pd" \
        || die "kubectl apply of PluginDefinition failed"
    ok "applied PluginDefinition 'rootstock' in ns ${DEMO_ORG}"

    if wait_ready plugindefinition rootstock "$DEMO_ORG"; then
        ok "PluginDefinition rootstock is READY=True"
    else
        die "PluginDefinition rootstock did not become READY within ${READY_TIMEOUT}s"
    fi
}

# =============================================================================
# Phase 5: optional full Plugin E2E (--with-plugin)
# =============================================================================
run_plugin_e2e() {
    log "Phase 5: full Plugin E2E (--with-plugin)"

    # 1. Make the rootstock image available on the remote cluster's nodes.
    if ! docker image inspect "$ROOTSTOCK_IMAGE" >/dev/null 2>&1; then
        die "image ${ROOTSTOCK_IMAGE} not found locally; build it first (e.g. 'make build' / 'make kind-up')"
    fi
    log "loading ${ROOTSTOCK_IMAGE} into kind cluster ${REMOTE_CLUSTER}"
    kind load docker-image "$ROOTSTOCK_IMAGE" --name "$REMOTE_CLUSTER" \
        || die "kind load of ${ROOTSTOCK_IMAGE} into ${REMOTE_CLUSTER} failed"
    ok "image loaded into remote cluster"

    # 2. Redfish Secret in the org namespace (Greenhouse resolves valueFrom.secret there).
    local redfish_pw="${REDFISH_PASSWORD:-redfish-test-password}"
    kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" create secret generic rootstock-redfish \
        --from-literal=redfish_password="$redfish_pw" \
        --dry-run=client -o yaml | kubectl --context "$ADMIN_CONTEXT" apply -f - \
        || die "failed to create rootstock-redfish secret"
    ok "created Secret rootstock-redfish in ns ${DEMO_ORG}"

    # 3. Apply the Plugin (temp copy with placeholders substituted).
    #    clusterName must match the onboarded remote cluster name (== REMOTE_CLUSTER).
    local plugin="${TMPDIR_RUN}/plugin.yaml"
    cp "${GH_TEST_DIR}/plugin.yaml" "$plugin"
    yq -i "
      .metadata.namespace = \"${DEMO_ORG}\" |
      .spec.clusterName = \"${REMOTE_CLUSTER}\" |
      (.spec.optionValues[] | select(.name == "image.repository").value) = "${ROOTSTOCK_IMAGE%:*}" |
      (.spec.optionValues[] | select(.name == "image.tag").value) = "${ROOTSTOCK_IMAGE##*:}"
    " "$plugin"
    kubectl --context "$ADMIN_CONTEXT" apply -f "$plugin" \
        || die "kubectl apply of Plugin failed"
    ok "applied Plugin 'rootstock' (cluster=${REMOTE_CLUSTER}, ns=${DEMO_ORG})"

    if wait_ready plugin rootstock "$DEMO_ORG"; then
        ok "Plugin rootstock is READY=True"
    else
        die "Plugin rootstock did not become READY within ${READY_TIMEOUT}s"
    fi

    # 4. Probe check against the deployed workload on the REMOTE cluster.
    probe_check
}

probe_check() {
    log "verifying /healthz and /probe on the deployed workload (remote cluster)"
    local ns="rootstock" release="rootstock" local_port="${LOCAL_PORT:-9220}"
    kubectl --context "$REMOTE_CONTEXT" -n "$ns" rollout status "deploy/${release}" --timeout=120s \
        || die "rootstock deployment never became ready on ${REMOTE_CONTEXT}"

    local pf_pid=""
    cleanup_pf() { [[ -n "$pf_pid" ]] && kill "$pf_pid" 2>/dev/null || true; }
    trap cleanup_pf RETURN

    kubectl --context "$REMOTE_CONTEXT" -n "$ns" port-forward "svc/${release}" "${local_port}:9220" \
        >/dev/null 2>&1 &
    pf_pid=$!

    local i
    for ((i = 0; i < 30; i++)); do
        curl -sf "http://localhost:${local_port}/healthz" >/dev/null 2>&1 && break
        sleep 1
    done
    curl -sf "http://localhost:${local_port}/healthz" >/dev/null \
        || die "/healthz never became ready"
    ok "/healthz OK"

    # Probe the in-cluster redfish mockup. The collector skips zero-valued
    # readings, so a matching metric line is a genuine end-to-end success.
    local target="${TARGET:-http://redfish-mockup.${ns}.svc.cluster.local:8000}"
    local body
    body="$(curl -sf "http://localhost:${local_port}/probe?target=${target}&labels=1")" \
        || die "/probe request failed"
    if echo "$body" | grep -Eqi 'pdu_|power|energy|apparent'; then
        ok "/probe returned real rootstock metrics"
    else
        warn "/probe output head:"; echo "$body" | head -n 20
        die "no expected metrics found in /probe output"
    fi
}

# =============================================================================
# Phase 6: teardown
# =============================================================================
teardown() {
    local rc=$?
    [[ -n "$TMPDIR_RUN" && -d "$TMPDIR_RUN" ]] && rm -rf "$TMPDIR_RUN"
    if $KEEP; then
        warn "--keep set: leaving kind clusters and ${GREENHOUSE_DIR} in place"
        return $rc
    fi
    log "Phase 6: teardown"
    kind delete cluster --name "$REMOTE_CLUSTER" >/dev/null 2>&1 || true
    kind delete cluster --name "$ADMIN_CLUSTER" >/dev/null 2>&1 || true
    docker rm -f registry >/dev/null 2>&1 || true
    rm -rf "$WORKDIR" || true
    ok "removed clusters, registry and workdir"
    return $rc
}

# =============================================================================
# Version resolution
# =============================================================================
# Resolve the chart version (authority: Chart.yaml) and app version (authority:
# VERSION.yaml), honouring CHART_VERSION / APP_VERSION / ROOTSTOCK_IMAGE
# overrides. Called once at the start of main() so every phase stays in lockstep.
resolve_versions() {
    log "resolving versions"

    if [[ -z "$CHART_VERSION" ]]; then
        CHART_VERSION="$(yq -r '.version' "${CHART_DIR}/Chart.yaml" 2>/dev/null)" \
            || die "failed to read .version from ${CHART_DIR}/Chart.yaml"
        [[ -n "$CHART_VERSION" && "$CHART_VERSION" != "null" ]] \
            || die "chart version is empty/null in ${CHART_DIR}/Chart.yaml"
        ok "chart version ${CHART_VERSION} (from Chart.yaml)"
    else
        ok "chart version ${CHART_VERSION} (CHART_VERSION override)"
    fi

    if [[ -z "$APP_VERSION" ]]; then
        APP_VERSION="$(yq -r '.version' "${ROOTSTOCK_ROOT}/VERSION.yaml" 2>/dev/null)" \
            || die "failed to read .version from VERSION.yaml"
        [[ -n "$APP_VERSION" && "$APP_VERSION" != "null" ]] \
            || die "app version is empty/null in VERSION.yaml"
        ok "app version ${APP_VERSION} (from VERSION.yaml)"
    else
        ok "app version ${APP_VERSION} (APP_VERSION override)"
    fi

    if [[ -z "$ROOTSTOCK_IMAGE" ]]; then
        ROOTSTOCK_IMAGE="rootstock:${APP_VERSION}"
        ok "image ${ROOTSTOCK_IMAGE} (derived from APP_VERSION)"
    else
        ok "image ${ROOTSTOCK_IMAGE} (ROOTSTOCK_IMAGE override)"
    fi
}

# =============================================================================
# main
# =============================================================================
main() {
    parse_args "$@"
    TMPDIR_RUN="$(mktemp -d "${TMPDIR:-/tmp}/rootstock-ghverify.XXXXXX")"
    trap teardown EXIT

    preflight
    resolve_versions
    fetch_greenhouse
    bring_up_greenhouse
    publish_chart
    apply_plugindefinition
    if $WITH_PLUGIN; then
        run_plugin_e2e
    else
        log "skipping Plugin E2E (pass --with-plugin to enable)"
    fi

    log "VERIFICATION PASSED"
}

main "$@"



