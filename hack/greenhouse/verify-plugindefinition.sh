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
#   3. `make setup-manager setup-demo` (brings up admin+remote kind clusters,
#      registry, demo org; the flaky dashboard UI step is skipped by default)
#   4. helm package + push the rootstock chart to the local registry
#   5. apply the PluginDefinition (temp copy, repository rewritten) and
#      assert READY=True   <-- primary pass/fail signal
#
# Opt-in full end-to-end (--with-plugin):
#   6. load the rootstock image into the remote cluster, create the redfish
#      Secret, apply a Plugin and assert it is Ready, deploy the bundled DMTF
#      Redfish mockup (unless TARGET is overridden), then port-forward and
#      assert /healthz + /probe emit real metrics.
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
# Name under which the remote cluster is *onboarded* into Greenhouse (the
# greenhouse.sap/Cluster object + its kubeconfig Secret). Greenhouse's
# setup-demo registers it from the kube-context name, i.e. "kind-<cluster>",
# which differs from the bare kind cluster name used for `kind load`/contexts.
# This is what a Plugin's spec.clusterName must reference.
REMOTE_ONBOARDED_NAME="${REMOTE_ONBOARDED_NAME:-${REMOTE_CONTEXT}}"
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
#   - ROOTSTOCK_IMAGE          : the local build tag on the host docker daemon
#                                (source image), e.g. rootstock:0.1.0.
#   - ROOTSTOCK_REGISTRY_IMAGE : the reference the Plugin/HelmRelease pulls from.
#       Greenhouse's demo org rewrites images on known upstream domains
#       (docker.io, gcr.io, ghcr.io, quay.io, registry.k8s.io) to its on-demand
#       zot mirror sub-paths. A bare/docker.io "rootstock" ref would be rewritten
#       to registry:5000/greenhouse-dockerhub-mirror/library/rootstock and zot
#       would try (and fail) to sync it from Docker Hub. Pointing at the local
#       registry path (REGISTRY_INCLUSTER/rootstock) is NOT a known upstream
#       domain, so it passes through un-rewritten and resolves to the OCI image
#       we push into zot in run_plugin_e2e().
ROOTSTOCK_IMAGE="${ROOTSTOCK_IMAGE:-}"
ROOTSTOCK_REGISTRY_IMAGE="${ROOTSTOCK_REGISTRY_IMAGE:-}"

# Image used by the chart's Helm test hook (templates/tests/test-connection.yaml).
# The chart defaults this to curlimages/curl on Docker Hub, which the Greenhouse
# dev-env registry cannot mirror (its zot refuses Docker-format upstream
# manifests: "http.compat docker2s2 not enabled"). We therefore re-publish it as
# an OCI image under a plain local-registry path (NOT the dockerhub mirror
# sub-path, which is sync-only and rejects pushes) and point the Plugin's
# testConnection.image.* optionValues at it. See run_plugin_e2e().
#   - TESTCONN_SOURCE_IMAGE   : upstream image pulled once at push time.
#   - TESTCONN_REGISTRY_IMAGE : in-cluster ref the test pod pulls from.
TESTCONN_SOURCE_IMAGE="${TESTCONN_SOURCE_IMAGE:-curlimages/curl:8.11.1}"
TESTCONN_REGISTRY_IMAGE="${TESTCONN_REGISTRY_IMAGE:-}"

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
                  create the Redfish Secret, apply a Plugin and assert it is
                  Ready, deploy the bundled DMTF Redfish mockup (unless TARGET
                  is set), then verify /healthz + /probe emit real metrics.
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
  TARGET          (default: http://redfish-mockup.rootstock.svc.cluster.local:8000)
                  Redfish endpoint the collector probes under --with-plugin.
                  When set, the bundled DMTF mockup is NOT deployed and the
                  endpoint must already be reachable from the remote cluster.
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
    # Greenhouse's aggregate `make setup` = setup-manager + setup-dashboard +
    # setup-demo, run in that order. The dashboard (UI) step is irrelevant to
    # PluginDefinition verification and is flaky to bring up locally; when it
    # fails, make aborts *before* setup-demo and the demo Organization/registry
    # are never created. So by default we run only the targets we need --
    # setup-manager (clusters, cert-manager, CRDs, controller, webhook) and
    # setup-demo (zot registry, demo Organization) -- skipping the dashboard.
    # Override GREENHOUSE_SETUP_TARGETS to change this (e.g. "setup" for the full
    # upstream flow including the dashboard).
    local setup_targets="${GREENHOUSE_SETUP_TARGETS:-setup-manager setup-demo}"
    log "running: make ${setup_targets}"
    (
        cd "$GREENHOUSE_DIR"
        # shellcheck disable=SC2086 # intentional word-splitting of target list
        make ${setup_targets}
    ) || die "Greenhouse 'make ${setup_targets}' failed (see output above)"

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

    # Invalidate any cached Flux HelmChart artifact for this version. Flux keys
    # its OCI chart cache by VERSION, so re-pushing an edited chart under the
    # SAME version (the common case when iterating with --keep) does NOT trigger
    # a re-pull -- Greenhouse would keep rendering the stale artifact and miss
    # our edits (e.g. the parameterized test-connection image). Nudging the
    # HelmRepository to reconcile and deleting the HelmChart (Greenhouse
    # recreates it) forces a fresh pull of the digest we just pushed.
    # Best-effort: on a clean run these objects do not exist yet, which is fine.
    local hr_names
    hr_names="$(kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" get helmrepository \
        -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)"
    if [[ -n "$hr_names" ]]; then
        local hr
        for hr in $hr_names; do
            kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" annotate helmrepository "$hr" \
                "reconcile.fluxcd.io/requestedAt=$(date +%s)" --overwrite >/dev/null 2>&1 || true
        done
    fi
    if kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" get helmchart "rootstock-${CHART_VERSION}" >/dev/null 2>&1; then
        kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" delete helmchart "rootstock-${CHART_VERSION}" >/dev/null 2>&1 || true
        ok "invalidated stale Flux HelmChart cache (rootstock-${CHART_VERSION})"
    fi
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
# Push an image into the Greenhouse local registry (zot) as an OCI image.
#
# Why not `kind load` + pullPolicy:Never? Under the Greenhouse demo org the
# HelmRelease image reference is resolved from the registry (see the
# ROOTSTOCK_REGISTRY_IMAGE comment), not the node's containerd store, so the
# image must live in zot. zot (the dev-env build) rejects Docker schema2
# manifests (`manifest invalid`) and only accepts OCI manifests, which the
# default `docker` buildx driver / `docker push` cannot produce. We therefore
# build+push via a throwaway `docker-container` buildx builder on the host
# network (so it can reach ${REGISTRY_HOST}) with an insecure (HTTP) registry
# and OCI output.
#
# Two modes:
#   push_image_oci <dest_host>                  # build from the repo Dockerfile
#   push_image_oci <dest_host> <from_image>     # trivial "FROM <from_image>"
#                                               # (re-publish an upstream image
#                                               #  as OCI, e.g. the test curl)
# dest_host is the destination reference as the HOST sees it (REGISTRY_HOST/...).
#
# Builder lifecycle: a single dedicated host-network docker-container builder is
# created once (idempotently) and reused across calls, then removed in
# teardown(). Both the host-network driver-opt and the docker-container driver
# are REQUIRED and were verified empirically:
#   - the plain `docker` driver cannot push to an insecure registry;
#   - a docker-container builder WITHOUT network=host cannot reach
#     localhost:5000 (it resolves ::1 in its own netns and times out);
#   - plain `docker push` / `crane push` of a `docker save` tarball are rejected
#     by zot with `manifest invalid` because they carry Docker schema2 media
#     types -- only buildkit's native OCI output is accepted.
OCI_BUILDER="rootstock-ghverify-oci"

# Create the shared OCI builder once; safe to call repeatedly.
ensure_oci_builder() {
    if docker buildx inspect "$OCI_BUILDER" >/dev/null 2>&1; then
        return
    fi
    docker buildx create --name "$OCI_BUILDER" \
        --driver docker-container \
        --driver-opt network=host \
        --buildkitd-flags '--allow-insecure-entitlement network.host' \
        >/dev/null 2>&1 \
        || die "failed to create buildx builder ${OCI_BUILDER}"
    docker buildx inspect --bootstrap "$OCI_BUILDER" >/dev/null 2>&1 \
        || { docker buildx rm "$OCI_BUILDER" >/dev/null 2>&1 || true; \
             die "failed to bootstrap buildx builder ${OCI_BUILDER}"; }
}

push_image_oci() {
    local dest_host="$1"
    local from_image="${2:-}"
    local ctx dockerfile build_args=()

    log "pushing OCI image ${dest_host} into local registry"
    ensure_oci_builder

    if [[ -n "$from_image" ]]; then
        # Re-publish an upstream image as OCI via a one-line FROM Dockerfile.
        ctx="$(mktemp -d "${TMPDIR:-/tmp}/ghverify-oci.XXXXXX")"
        dockerfile="${ctx}/Dockerfile"
        printf 'FROM %s\n' "$from_image" > "$dockerfile"
    else
        # Build straight from the repo Dockerfile.
        ctx="${ROOTSTOCK_ROOT}"
        dockerfile="${ROOTSTOCK_ROOT}/Dockerfile"
        build_args=(--build-arg "VERSION=${APP_VERSION}")
    fi

    docker buildx --builder "$OCI_BUILDER" build \
        "${build_args[@]}" \
        --platform "${BUILD_PLATFORM:-linux/arm64}" \
        --provenance=false \
        --output "type=registry,registry.insecure=true,name=${dest_host}" \
        -f "$dockerfile" "$ctx" \
        || { [[ -n "$from_image" ]] && rm -rf "$ctx"; \
             die "OCI build/push of ${dest_host} failed"; }

    [[ -n "$from_image" ]] && rm -rf "$ctx"
    ok "pushed OCI image ${dest_host}"
}

# =============================================================================
# Phase 5: optional full Plugin E2E (--with-plugin)
# =============================================================================
run_plugin_e2e() {
    log "Phase 5: full Plugin E2E (--with-plugin)"

    # 1. Make the rootstock image available to the remote cluster via the local
    #    registry (zot). Greenhouse resolves the HelmRelease image from the
    #    registry, so a `kind load` into node containerd is not enough -- we push
    #    an OCI image and point the Plugin at REGISTRY_INCLUSTER/rootstock.
    if ! docker image inspect "$ROOTSTOCK_IMAGE" >/dev/null 2>&1; then
        die "image ${ROOTSTOCK_IMAGE} not found locally; build it first (e.g. 'make docker-build' / 'make kind-up')"
    fi
    # Push using the HOST-visible registry address; the Plugin references the
    # in-cluster address (REGISTRY_INCLUSTER) that resolves to the same zot.
    push_image_oci "${REGISTRY_HOST}/rootstock:${ROOTSTOCK_IMAGE##*:}"
    ok "image available in registry as ${ROOTSTOCK_REGISTRY_IMAGE}"

    # 1b. The chart's Helm test hook (templates/tests/test-connection.yaml) curls
    #     the pod's health endpoints. Greenhouse pre-validates EVERY chart image
    #     -- including test-hook images -- before creating the HelmRelease, so the
    #     default curlimages/curl must resolve too. The dev-env registry cannot
    #     mirror it from Docker Hub (docker2s2 disabled), so re-publish it as OCI
    #     under a plain local path and point the Plugin's testConnection.image.*
    #     at it. (TESTCONN_REGISTRY_IMAGE resolved in resolve_versions().)
    push_image_oci \
        "${REGISTRY_HOST}/${TESTCONN_REGISTRY_IMAGE#*/}" \
        "$TESTCONN_SOURCE_IMAGE"
    ok "test-connection image available in registry as ${TESTCONN_REGISTRY_IMAGE}"

    # 2. Redfish Secret in the org namespace (Greenhouse resolves valueFrom.secret there).
    local redfish_pw="${REDFISH_PASSWORD:-redfish-test-password}"
    kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" create secret generic rootstock-redfish \
        --from-literal=redfish_password="$redfish_pw" \
        --dry-run=client -o yaml | kubectl --context "$ADMIN_CONTEXT" apply -f - \
        || die "failed to create rootstock-redfish secret"
    ok "created Secret rootstock-redfish in ns ${DEMO_ORG}"

    # 3. Apply the Plugin (temp copy with placeholders substituted).
    #    spec.clusterName must match the *onboarded* Greenhouse Cluster name
    #    (REMOTE_ONBOARDED_NAME, "kind-<remote>"), not the bare kind cluster name.
    local plugin="${TMPDIR_RUN}/plugin.yaml"
    cp "${GH_TEST_DIR}/plugin.yaml" "$plugin"
    # Normalize the Plugin into the schema served by the pinned Greenhouse
    # (GREENHOUSE_REF, default v0.17.2): that CRD requires spec.pluginDefinitionRef
    # ({kind,name}) and rejects the legacy spec.pluginDefinition / spec.disabled
    # fields under strict decoding. The committed source manifest keeps the
    # human-facing shape; we rewrite it here so the apply matches the live CRD.
    # We also repoint image.repository/tag at the local registry (zot) and flip
    # image.pullPolicy away from the source's "Never" (which assumes a node
    # containerd preload) to "IfNotPresent" so the kubelet pulls from the
    # registry we pushed to in step 1.
    yq -i "
      .metadata.namespace = \"${DEMO_ORG}\" |
      .spec.clusterName = \"${REMOTE_ONBOARDED_NAME}\" |
      .spec.pluginDefinitionRef = {\"kind\": \"PluginDefinition\", \"name\": \"rootstock\"} |
      del(.spec.pluginDefinition) |
      del(.spec.disabled) |
      (.spec.optionValues[] | select(.name == \"image.pullPolicy\").value) = \"IfNotPresent\" |
      (.spec.optionValues[] | select(.name == \"image.repository\").value) = \"${ROOTSTOCK_REGISTRY_IMAGE%:*}\" |
      (.spec.optionValues[] | select(.name == \"image.tag\").value) = \"${ROOTSTOCK_REGISTRY_IMAGE##*:}\" |
      .spec.optionValues += [
        {\"name\": \"testConnection.image.repository\", \"value\": \"${TESTCONN_REGISTRY_IMAGE%:*}\"},
        {\"name\": \"testConnection.image.tag\", \"value\": \"${TESTCONN_REGISTRY_IMAGE##*:}\"}
      ]
    " "$plugin"
    kubectl --context "$ADMIN_CONTEXT" apply -f "$plugin" \
        || die "kubectl apply of Plugin failed"
    ok "applied Plugin 'rootstock' (cluster=${REMOTE_ONBOARDED_NAME}, ns=${DEMO_ORG})"

    if wait_ready plugin rootstock "$DEMO_ORG"; then
        ok "Plugin rootstock is READY=True"
    else
        die "Plugin rootstock did not become READY within ${READY_TIMEOUT}s"
    fi

    # 4. Provision the Redfish target the collector will scrape, then probe.
    #    If the caller supplied TARGET, assume it is already reachable from the
    #    remote cluster and skip deploying the bundled DMTF mockup.
    if [[ -n "${TARGET:-}" ]]; then
        log "TARGET override set (${TARGET}); skipping in-cluster mockup provisioning"
    else
        deploy_mockup
    fi

    # 5. Probe check against the deployed workload on the REMOTE cluster.
    probe_check
}

# Build + load the DMTF Redfish mockup (pre-loaded with rootstock's PDU dataset)
# and deploy it into the workload namespace on the REMOTE cluster, so the
# collector can scrape it via in-cluster DNS. Mirrors the Makefile 'kind-mockup'
# target, retargeted at the Greenhouse remote cluster.
deploy_mockup() {
    local ns="rootstock"
    log "building and loading DMTF redfish mockup image"
    docker build -t rootstock-mockup:latest \
        -f "${ROOTSTOCK_ROOT}/hack/kind/Dockerfile.mockup" "$ROOTSTOCK_ROOT" >/dev/null \
        || die "failed to build rootstock-mockup image"
    kind load docker-image rootstock-mockup:latest --name "$REMOTE_CLUSTER" \
        || die "kind load of rootstock-mockup into ${REMOTE_CLUSTER} failed"

    log "deploying redfish mockup into ns ${ns} on ${REMOTE_CONTEXT}"
    kubectl --context "$REMOTE_CONTEXT" create namespace "$ns" \
        --dry-run=client -o yaml | kubectl --context "$REMOTE_CONTEXT" apply -f - \
        || die "failed to ensure namespace ${ns}"
    kubectl --context "$REMOTE_CONTEXT" -n "$ns" apply -f "${ROOTSTOCK_ROOT}/hack/kind/redfish-mockup.yaml" \
        || die "failed to apply redfish-mockup manifest"
    kubectl --context "$REMOTE_CONTEXT" -n "$ns" rollout status deploy/redfish-mockup --timeout=120s \
        || die "redfish-mockup never became ready on ${REMOTE_CONTEXT}"
    ok "redfish mockup deployed (redfish-mockup.${ns}.svc:8000)"
}

probe_check() {
    log "verifying /healthz and /probe on the deployed workload (remote cluster)"
    local ns="rootstock" release="rootstock" local_port="${LOCAL_PORT:-9220}"
    kubectl --context "$REMOTE_CONTEXT" -n "$ns" rollout status "deploy/${release}" --timeout=120s \
        || die "rootstock deployment never became ready on ${REMOTE_CONTEXT}"

    local pf_pid=""
    cleanup_pf() { [[ -n "${pf_pid:-}" ]] && kill "$pf_pid" 2>/dev/null || true; }
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
    # Always remove the shared OCI buildx builder (a host-side resource, not part
    # of the kept cluster state).
    docker buildx rm "$OCI_BUILDER" >/dev/null 2>&1 || true
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

    # Registry reference the Plugin pulls from (see the ROOTSTOCK_REGISTRY_IMAGE
    # comment near the top): the in-cluster local registry path, which Greenhouse
    # does NOT rewrite through its upstream mirror. Keep the image's tag.
    if [[ -z "$ROOTSTOCK_REGISTRY_IMAGE" ]]; then
        ROOTSTOCK_REGISTRY_IMAGE="${REGISTRY_INCLUSTER}/rootstock:${ROOTSTOCK_IMAGE##*:}"
        ok "registry image ${ROOTSTOCK_REGISTRY_IMAGE} (derived)"
    else
        ok "registry image ${ROOTSTOCK_REGISTRY_IMAGE} (ROOTSTOCK_REGISTRY_IMAGE override)"
    fi

    # Test-hook image reference the Plugin pulls from: the local registry path,
    # keyed by the source image's name/tag (see the TESTCONN_* comment near the
    # top). The leading registry/host portion of the source is dropped so a
    # docker.io ref like curlimages/curl:8.11.1 maps to
    # ${REGISTRY_INCLUSTER}/curlimages/curl:8.11.1.
    if [[ -z "$TESTCONN_REGISTRY_IMAGE" ]]; then
        # Preserve the repo path minus any registry host. curlimages/curl has no
        # host component, so keep it as-is; a host like docker.io/foo/bar keeps
        # foo/bar. Detect a host by a dot/colon in the first path segment.
        local tc_repo="${TESTCONN_SOURCE_IMAGE%:*}"
        local tc_tag="${TESTCONN_SOURCE_IMAGE##*:}"
        local first="${tc_repo%%/*}"
        if [[ "$tc_repo" == */* && ( "$first" == *.* || "$first" == *:* || "$first" == "localhost" ) ]]; then
            tc_repo="${tc_repo#*/}"                    # drop registry host segment
        fi
        TESTCONN_REGISTRY_IMAGE="${REGISTRY_INCLUSTER}/${tc_repo}:${tc_tag}"
        ok "test-connection image ${TESTCONN_REGISTRY_IMAGE} (derived)"
    else
        ok "test-connection image ${TESTCONN_REGISTRY_IMAGE} (TESTCONN_REGISTRY_IMAGE override)"
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



