#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# verify-plugindefinition.sh — verify the rootstock Greenhouse PluginDefinition.
#
# Requires a running Greenhouse-in-kind environment (run setup_greenhouse.sh first).
# Default: preflight → helm package+push → apply PluginDefinition and assert READY=True.
# --with-plugin: also load image, create Redfish Secret, apply Plugin, deploy mockup,
#               assert /healthz + /probe emit real metrics.
# Committed plugindefinition.yaml is NEVER mutated; all rewrites happen on temp copies.
set -euo pipefail

# --- Configuration (all overridable) -----------------------------------------
# Cluster / org / context names (must match what setup_greenhouse.sh created).
admin_cluster="greenhouse-admin"
remote_cluster="greenhouse-remote"
admin_context="kind-${admin_cluster}"
# Greenhouse onboards the remote cluster under its kube-context name ("kind-<cluster>"),
# which is what Plugin spec.clusterName must reference.
remote_context="kind-${remote_cluster}"

DEMO_ORG="demo"

# Local registry provided by Greenhouse's hack/registry/setup.sh (zot).
# REGISTRY_HOST: host-visible address (helm push); REGISTRY_INCLUSTER: in-cluster address (Flux).
REGISTRY_HOST="${REGISTRY_HOST:-localhost:5000}"
REGISTRY_INCLUSTER="${REGISTRY_INCLUSTER:-registry:5000}"
# OCI path (without the oci:// scheme) under which the chart is pushed.
REGISTRY_CHART_REPO="${REGISTRY_CHART_REPO:-charts}"

# rootstock image coordinates. Default tag comes from VERSION.yaml via resolve_versions().
# ROOTSTOCK_IMAGE: local build tag on the host docker daemon (source image).
# ROOTSTOCK_REGISTRY_IMAGE: in-cluster ref the Plugin/HelmRelease pulls from; must point at
#   REGISTRY_INCLUSTER (not docker.io etc.) so Greenhouse's mirror rewrite doesn't break it.
ROOTSTOCK_IMAGE="${ROOTSTOCK_IMAGE:-}"
ROOTSTOCK_REGISTRY_IMAGE="${ROOTSTOCK_REGISTRY_IMAGE:-}"

# Helm test hook image. The chart defaults to curlimages/curl (Docker Hub), which the
# dev-env zot registry cannot mirror (docker2s2 disabled). We re-publish it as OCI under
# a plain local-registry path and override testConnection.image.* in the Plugin.
TESTCONN_SOURCE_IMAGE="${TESTCONN_SOURCE_IMAGE:-curlimages/curl:8.11.1}"
TESTCONN_REGISTRY_IMAGE="${TESTCONN_REGISTRY_IMAGE:-}"

# Chart version packaged and pushed. Authority: k8s_configuration/chart/Chart.yaml `version`.
# Override via CHART_VERSION (repackages current chart source under that version number).
CHART_VERSION="${CHART_VERSION:-}"
# App/image version. Authority: VERSION.yaml `version`.
APP_VERSION="${APP_VERSION:-}"

# Polling.
READY_TIMEOUT="${READY_TIMEOUT:-60}"   # seconds to wait for PluginDefinition/Plugin READY
POLL_INTERVAL="${POLL_INTERVAL:-5}"

# Flags (set by argument parsing).
WITH_PLUGIN=false

# --- Paths --------------------------------------------------------------------
rootstock_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
chart_dir="${rootstock_root}/k8s_configuration/chart"
plugin_def="${rootstock_root}/k8s_configuration/plugindefinition.yaml"
greenhouse_test_dir="${rootstock_root}/k8s_configuration/greenhouse-test"
workdir="${rootstock_root}/tmp/greenhouse/verify"

# =============================================================================
# main
# =============================================================================
main() {
    parse_args "$@"
    workdir="$(mktemp -d "${TMPDIR:-/tmp}/rootstock-ghverify.XXXXXX")"
    trap teardown EXIT

    preflight
    resolve_versions
    publish_chart
    apply_plugindefinition
    if $WITH_PLUGIN; then
        run_plugin_e2e
    else
        log "skipping Plugin E2E (pass --with-plugin to enable)"
    fi

    log "VERIFICATION PASSED"
}

# --- Logging helpers ----------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  OK\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  !!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
    cat <<EOF
Usage: $(basename "$0") [--with-plugin] [-h|--help]

  --with-plugin   Also run the full E2E: load image into the remote cluster,
                  create the Redfish Secret, apply a Plugin and assert it is
                  Ready, deploy the bundled DMTF Redfish mockup (unless TARGET
                  is set), then verify /healthz + /probe emit real metrics.
  -h, --help      Show this help.

Key environment overrides:
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
    [[ -d "$chart_dir" ]] || die "rootstock chart not found at ${chart_dir}"
    [[ -f "$plugin_def" ]] || die "plugindefinition.yaml not found at ${plugin_def}"
    docker inspect registry >/dev/null 2>&1 \
        || die "local registry container not running; run setup_greenhouse.sh first"
    ok "all tools present and docker daemon reachable"
}

# =============================================================================
# Phase 1: publish the rootstock chart to the local registry
# =============================================================================
publish_chart() {
    log "Phase 1: package + push rootstock chart to oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO}"

    # Confirm the registry container exists (started by setup_greenhouse.sh).
    docker inspect registry >/dev/null 2>&1 \
        || die "local registry container not running; run setup_greenhouse.sh first"

    local pkgdir="${workdir}/chart"
    mkdir -p "$pkgdir"
    # --version overrides the chart version without mutating Chart.yaml.
    helm package "$chart_dir" --version "$CHART_VERSION" -d "$pkgdir" >/dev/null \
        || die "helm package failed"
    local tgz="${pkgdir}/rootstock-${CHART_VERSION}.tgz"
    [[ -f "$tgz" ]] || die "expected packaged chart ${tgz} not found"
    ok "packaged $(basename "$tgz")"

    # Push over plain HTTP (zot is HTTP on localhost:5000).
    helm push "$tgz" "oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO}" --plain-http \
        || die "helm push to oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO} failed"
    ok "pushed chart to oci://${REGISTRY_HOST}/${REGISTRY_CHART_REPO}/rootstock:${CHART_VERSION}"

    # Flux keys its OCI chart cache by version; re-pushing the same version without
    # invalidating the HelmChart causes Greenhouse to render the stale artifact.
    # Nudge the HelmRepository and delete the HelmChart to force a fresh pull.
    local hr_names
    hr_names="$(kubectl --context "$admin_context" -n "$DEMO_ORG" get helmrepository \
        -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)"
    if [[ -n "$hr_names" ]]; then
        local hr
        for hr in $hr_names; do
            kubectl --context "$admin_context" -n "$DEMO_ORG" annotate helmrepository "$hr" \
                "reconcile.fluxcd.io/requestedAt=$(date +%s)" --overwrite >/dev/null 2>&1 || true
        done
    fi
    if kubectl --context "$admin_context" -n "$DEMO_ORG" get helmchart "rootstock-${CHART_VERSION}" >/dev/null 2>&1; then
        kubectl --context "$admin_context" -n "$DEMO_ORG" delete helmchart "rootstock-${CHART_VERSION}" >/dev/null 2>&1 || true
        ok "invalidated stale Flux HelmChart cache (rootstock-${CHART_VERSION})"
    fi
}

# =============================================================================
# Phase 2: apply PluginDefinition and assert READY=True  (primary signal)
# =============================================================================
wait_ready() {
    # wait_ready <kind> <name> <namespace-or-empty>
    local kind="$1" name="$2" ns="${3:-}"
    local nsargs=()
    [[ -n "$ns" ]] && nsargs=(-n "$ns")
    local deadline=$(( $(date +%s) + READY_TIMEOUT ))
    local status=""
    while (( $(date +%s) < deadline )); do
        status="$(kubectl --context "$admin_context" "${nsargs[@]}" get "$kind" "$name" \
            -o jsonpath='{.status.statusConditions.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)"
        if [[ "$status" == "True" ]]; then
            return 0
        fi
        printf '    waiting for %s/%s Ready (current: %s)\n' "$kind" "$name" "${status:-<none>}"
        sleep "$POLL_INTERVAL"
    done
    warn "timed out waiting for ${kind}/${name} Ready; last status: ${status:-<none>}"
    kubectl --context "$admin_context" "${nsargs[@]}" get "$kind" "$name" -o yaml 2>/dev/null \
        | yq '.status' - || true
    return 1
}

apply_plugindefinition() {
    log "Phase 2: apply PluginDefinition and assert READY=True"

    # Operate on a TEMP COPY — never mutate the committed plugindefinition.yaml.
    # Rewrite helmChart.repository to the in-cluster address, pin versions to CHART_VERSION.
    # NOTE: PluginDefinition is namespaced in Greenhouse; it must live in the org namespace
    # (${DEMO_ORG}) or the reconciler won't find the org and will never become Ready.
    local pd="${workdir}/plugindefinition.yaml"
    cp "$plugin_def" "$pd"
    yq -i "
      .metadata.namespace = \"${DEMO_ORG}\" |
      .spec.version = \"${CHART_VERSION}\" |
      .spec.helmChart.repository = \"oci://${REGISTRY_INCLUSTER}/${REGISTRY_CHART_REPO}\" |
      .spec.helmChart.version = \"${CHART_VERSION}\"
    " "$pd"
    ok "temp PluginDefinition: namespace=${DEMO_ORG}, version=${CHART_VERSION}, repository=oci://${REGISTRY_INCLUSTER}/${REGISTRY_CHART_REPO}"

    kubectl --context "$admin_context" -n "$DEMO_ORG" apply -f "$pd" \
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
# Why not `kind load` + pullPolicy:Never? The HelmRelease image reference is
# resolved from the registry, so the image must live in zot. zot only accepts
# OCI manifests (`manifest invalid` on Docker schema2), so we use a throwaway
# docker-container buildx builder on the host network with an insecure registry.
#
# Two modes:
#   push_image_oci <dest_host>              # build from the repo Dockerfile
#   push_image_oci <dest_host> <from_image> # re-publish an upstream image as OCI
# dest_host is the HOST-visible registry address (REGISTRY_HOST/...).
#
# Builder notes: docker-container driver + network=host are both required:
#   - the plain `docker` driver cannot push to an insecure registry;
#   - without network=host the builder cannot reach localhost:5000.
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
    local ctx dockerfile
    local build_extra_args

    log "pushing OCI image ${dest_host} into local registry"
    ensure_oci_builder

    if [[ -n "$from_image" ]]; then
        # Re-publish an upstream image as OCI via a one-line FROM Dockerfile.
        ctx="$(mktemp -d "${TMPDIR:-/tmp}/ghverify-oci.XXXXXX")"
        dockerfile="${ctx}/Dockerfile"
        printf 'FROM %s\n' "$from_image" > "$dockerfile"
        build_extra_args=""
    else
        # Build straight from the repo Dockerfile.
        ctx="${rootstock_root}"
        dockerfile="${rootstock_root}/Dockerfile"
        build_extra_args="--build-arg VERSION=${APP_VERSION}"
    fi

    docker buildx --builder "$OCI_BUILDER" build \
        "${build_extra_args}" \
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
# Phase 3: optional full Plugin E2E (--with-plugin)
# =============================================================================
run_plugin_e2e() {
    log "Phase 3: full Plugin E2E (--with-plugin)"

    # 1. Make the rootstock image available to the remote cluster via the local
    #    registry (zot). Greenhouse resolves the HelmRelease image from the
    #    registry, so a `kind load` into node containerd is not enough -- we push
    #    an OCI image and point the Plugin at REGISTRY_INCLUSTER/rootstock.
    if ! docker image inspect "$ROOTSTOCK_IMAGE" >/dev/null 2>&1; then
        die "image ${ROOTSTOCK_IMAGE} not found locally; build it first (e.g. 'make docker-build' / 'make kind-up')"
    fi
    # Push using the HOST-visible address; the Plugin references REGISTRY_INCLUSTER
    # (same zot, but reachable in-cluster).
    push_image_oci "${REGISTRY_HOST}/rootstock:${ROOTSTOCK_IMAGE##*:}"
    ok "image available in registry as ${ROOTSTOCK_REGISTRY_IMAGE}"

    # Re-publish the test-connection curl image as OCI; dev-env zot can't mirror
    # Docker Hub (docker2s2 disabled), so override testConnection.image.* in the Plugin.
    push_image_oci \
        "${REGISTRY_HOST}/${TESTCONN_REGISTRY_IMAGE#*/}" \
        "$TESTCONN_SOURCE_IMAGE"
    ok "test-connection image available in registry as ${TESTCONN_REGISTRY_IMAGE}"

    # 2. Wait for Greenhouse to see the remote cluster nodes as ready.
    #    When clusters are freshly created, there's a race where nodes become Ready
    #    but Greenhouse hasn't updated its cluster status yet, causing Plugin deployment to fail.
    log "waiting for Greenhouse to recognize cluster nodes as ready"
    local cluster_wait_timeout=300
    local cluster_wait_elapsed=0
    local cluster_refresh_attempts=0

    while [[ $cluster_wait_elapsed -lt $cluster_wait_timeout ]]; do
        local all_nodes_ready payload_schedulable

        all_nodes_ready=$(
          kubectl --context "$admin_context" get cluster "$remote_context" -n "$DEMO_ORG" \
            -o jsonpath='{.status.statusConditions.conditions[?(@.type=="AllNodesReady")].status}' 2>/dev/null \
          || echo "False"
        )
        payload_schedulable=$(
          kubectl --context "$admin_context" get cluster "$remote_context" -n "$DEMO_ORG" \
            -o jsonpath='{.status.statusConditions.conditions[?(@.type=="PayloadSchedulable")].status}' 2>/dev/null \
          || echo "False"
        )

        if [[ "$all_nodes_ready" == "True" && "$payload_schedulable" == "True" ]]; then
            ok "cluster nodes are ready and schedulable in Greenhouse"
            break
        fi

        # If nodes are still not ready after 60s, try to force a refresh by annotating the cluster
        if [[ $cluster_wait_elapsed -eq 60 && $cluster_refresh_attempts -eq 0 ]]; then
            echo
            log "forcing cluster status refresh..."
            kubectl --context "$admin_context" annotate cluster "$remote_context" -n "$DEMO_ORG" \
                greenhouse.sap/force-refresh="$(date +%s)" --overwrite >/dev/null 2>&1 || true
            cluster_refresh_attempts=1
            echo -n "    continuing to wait"
        elif [[ $cluster_wait_elapsed -eq 0 ]]; then
            echo -n "    waiting for cluster readiness (current: AllNodesReady=$all_nodes_ready, PayloadSchedulable=$payload_schedulable)"
        else
            echo -n "."
        fi

        sleep 5
        cluster_wait_elapsed=$((cluster_wait_elapsed + 5))
    done
    echo  # newline after dots

    # Final check - if still not ready, fail fast instead of proceeding
    local final_all_nodes_ready final_payload_schedulable

    final_all_nodes_ready=$(
      kubectl --context "$admin_context" get cluster "$remote_context" -n "$DEMO_ORG" \
        -o jsonpath='{.status.statusConditions.conditions[?(@.type=="AllNodesReady")].status}' 2>/dev/null \
      || echo "False"
      )
    final_payload_schedulable=$(
      kubectl --context "$admin_context" get cluster "$remote_context" -n "$DEMO_ORG" \
        -o jsonpath='{.status.statusConditions.conditions[?(@.type=="PayloadSchedulable")].status}' 2>/dev/null \
      || echo "False"
    )

    if [[ "$final_all_nodes_ready" != "True" || "$final_payload_schedulable" != "True" ]]; then
        die "cluster ${remote_context} is not ready after ${cluster_wait_timeout}s (AllNodesReady=${final_all_nodes_ready}, PayloadSchedulable=${final_payload_schedulable})"
    fi

    # 3. Redfish Secret in the org namespace (Greenhouse resolves valueFrom.secret there).
    local redfish_pw="${REDFISH_PASSWORD:-redfish-test-password}"
    kubectl --context "$admin_context" -n "$DEMO_ORG" create secret generic rootstock-redfish \
        --from-literal=redfish_password="$redfish_pw" \
        --dry-run=client -o yaml | kubectl --context "$admin_context" apply -f - \
        || die "failed to create rootstock-redfish secret"
    ok "created Secret rootstock-redfish in ns ${DEMO_ORG}"

    # 4. Apply the Plugin on a temp copy. Normalize to the pinned Greenhouse CRD schema
    # (spec.pluginDefinitionRef, drop legacy spec.pluginDefinition/disabled), repoint
    # image.repository/tag at the local registry, and flip pullPolicy to IfNotPresent.
    local plugin="${workdir}/plugin.yaml"
    cp "${greenhouse_test_dir}/plugin.yaml" "$plugin"
    yq -i "
      .metadata.namespace = \"${DEMO_ORG}\" |
      .spec.clusterName = \"${remote_context}\" |
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
    kubectl --context "$admin_context" apply -f "$plugin" \
        || die "kubectl apply of Plugin failed"
    ok "applied Plugin 'rootstock' (cluster=${remote_context}, ns=${DEMO_ORG})"

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
        -f "${rootstock_root}/hack/kind/Dockerfile.mockup" "$rootstock_root" >/dev/null \
        || die "failed to build rootstock-mockup image"
    kind load docker-image rootstock-mockup:latest --name "$remote_cluster" \
        || die "kind load of rootstock-mockup into ${remote_cluster} failed"

    log "deploying redfish mockup into ns ${ns} on ${remote_context}"
    kubectl --context "$remote_context" create namespace "$ns" \
        --dry-run=client -o yaml | kubectl --context "$remote_context" apply -f - \
        || die "failed to ensure namespace ${ns}"
    kubectl --context "$remote_context" -n "$ns" apply -f "${rootstock_root}/hack/kind/redfish-mockup.yaml" \
        || die "failed to apply redfish-mockup manifest"
    kubectl --context "$remote_context" -n "$ns" rollout status deploy/redfish-mockup --timeout=120s \
        || die "redfish-mockup never became ready on ${remote_context}"
    ok "redfish mockup deployed (redfish-mockup.${ns}.svc:8000)"
}

probe_check() {
    log "verifying /healthz and /probe on the deployed workload (remote cluster)"
    local ns="rootstock" release="rootstock" local_port="${LOCAL_PORT:-9220}"
    kubectl --context "$remote_context" -n "$ns" rollout status "deploy/${release}" --timeout=120s \
        || die "rootstock deployment never became ready on ${remote_context}"

    local pf_pid=""
    cleanup_pf() {
      # shellcheck disable=SC2015
      [[ -n "${pf_pid:-}" ]] && kill "$pf_pid" 2>/dev/null || true
    }
    trap cleanup_pf RETURN

    kubectl --context "$remote_context" -n "$ns" port-forward "svc/${release}" "${local_port}:9220" \
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
# Phase 4: teardown
# =============================================================================
teardown() {
    local rc=$?
    [[ -n "$workdir" && -d "$workdir" ]] && rm -rf "$workdir"
    # Always remove the shared OCI buildx builder (a host-side resource, not part
    # of the kept cluster state).
    docker buildx rm "$OCI_BUILDER" >/dev/null 2>&1 || true
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
        CHART_VERSION="$(yq -r '.version' "${chart_dir}/Chart.yaml" 2>/dev/null)" \
            || die "failed to read .version from ${chart_dir}/Chart.yaml"
        [[ -n "$CHART_VERSION" && "$CHART_VERSION" != "null" ]] \
            || die "chart version is empty/null in ${chart_dir}/Chart.yaml"
        ok "chart version ${CHART_VERSION} (from Chart.yaml)"
    else
        ok "chart version ${CHART_VERSION} (CHART_VERSION override)"
    fi

    if [[ -z "$APP_VERSION" ]]; then
        APP_VERSION="$(yq -r '.version' "${rootstock_root}/VERSION.yaml" 2>/dev/null)" \
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

    # Registry image the Plugin pulls from; use REGISTRY_INCLUSTER so Greenhouse's
    # upstream-mirror rewrite doesn't intercept it.
    if [[ -z "$ROOTSTOCK_REGISTRY_IMAGE" ]]; then
        ROOTSTOCK_REGISTRY_IMAGE="${REGISTRY_INCLUSTER}/rootstock:${ROOTSTOCK_IMAGE##*:}"
        ok "registry image ${ROOTSTOCK_REGISTRY_IMAGE} (derived)"
    else
        ok "registry image ${ROOTSTOCK_REGISTRY_IMAGE} (ROOTSTOCK_REGISTRY_IMAGE override)"
    fi

    # Test-hook image; drop the registry host from the source ref so
    # curlimages/curl:8.11.1 maps to ${REGISTRY_INCLUSTER}/curlimages/curl:8.11.1.
    if [[ -z "$TESTCONN_REGISTRY_IMAGE" ]]; then
        local tc_repo="${TESTCONN_SOURCE_IMAGE%:*}"
        local tc_tag="${TESTCONN_SOURCE_IMAGE##*:}"
        local first="${tc_repo%%/*}"
        if [[ "$tc_repo" == */* && ( "$first" == *.* || "$first" == *:* || "$first" == "localhost" ) ]]; then
            tc_repo="${tc_repo#*/}"
        fi
        TESTCONN_REGISTRY_IMAGE="${REGISTRY_INCLUSTER}/${tc_repo}:${tc_tag}"
        ok "test-connection image ${TESTCONN_REGISTRY_IMAGE} (derived)"
    else
        ok "test-connection image ${TESTCONN_REGISTRY_IMAGE} (TESTCONN_REGISTRY_IMAGE override)"
    fi
}



main "$@"



