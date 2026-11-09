#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# start_plugin.sh — start the rootstock Greenhouse Plugin.
#
# Requires a running Greenhouse-in-kind environment (run setup_greenhouse.sh first).
# Default: preflight → helm package+push → apply PluginDefinition and assert READY=True.
# --with-plugin: also load image, create Redfish Secret, apply Plugin, and start deployment.
#
# Environment:
#   SNMP_CONFIG_FILE: SNMP config file path (default: files/snmp.yml)
# Committed plugindefinition.yaml is NEVER mutated; all rewrites happen on temp copies.
set -euo pipefail

# --- Configuration ------------------------------------------------------------
# Cluster / org / context names (must match what setup_greenhouse.sh created).
# Greenhouse onboards the remote cluster under its kube-context name
# ("kind-<cluster>"), which is what Plugin spec.clusterName must reference.
admin_cluster="greenhouse-admin"
remote_cluster="greenhouse-remote"
admin_context="kind-${admin_cluster}"
remote_context="kind-${remote_cluster}"

demo_org="demo"

# Local registry (zot) from Greenhouse's hack/registry/setup.sh.
# registry_host: host-visible (helm push); registry_in_cluster: in-cluster (Flux).
# registry_chart_repo: OCI path (sans oci://) the chart is pushed under.
registry_host="localhost:5000"
registry_in_cluster="registry:5000"
registry_chart_repo="charts"

# Name of the throwaway buildx builder used to push OCI images (see below).
oci_builder="rootstock-ghverify-oci"

# SNMP exporter config file, relative to the chart directory.
SNMP_CONFIG_FILE="${SNMP_CONFIG_FILE:-files/snmp.yml}"

# rootstock image. Defaults to the Makefile IMAGE default; override via env.
ROOTSTOCK_IMAGE="${ROOTSTOCK_IMAGE:-}"

# Helm test-hook image. zot can't mirror Docker Hub (docker2s2 disabled), so we
# re-publish it as OCI and override testConnection.image.* in the Plugin.
test_conn_image="curlimages/curl:8.11.1"

# SNMP exporter sidecar image. zot can't mirror quay.io either, so we re-publish
# it as OCI and override snmpExporter.image.* in the Plugin.
snmp_exporter_image="quay.io/prometheus/snmp-exporter:v0.26.0"

# Polling (seconds).
READY_TIMEOUT="${READY_TIMEOUT:-60}"
POLL_INTERVAL="${POLL_INTERVAL:-5}"

# Flags (set by parse_args).
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
  ROOTSTOCK_IMAGE (default: ghcr.io/cloudoperators/rootstock:dev-\$USER, the
                  Makefile IMAGE default) Image used by --with-plugin.
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
    log "Phase 1: package + push rootstock chart to oci://${registry_host}/${registry_chart_repo}"

    # Confirm the registry container exists (started by setup_greenhouse.sh).
    docker inspect registry >/dev/null 2>&1 \
        || die "local registry container not running; run setup_greenhouse.sh first"

    local pkgdir="${workdir}/chart"
    mkdir -p "$pkgdir"
    # chart_version mirrors Chart.yaml; pass it explicitly so the packaged tgz
    # name is deterministic.
    helm package "$chart_dir" --version "$chart_version" -d "$pkgdir" >/dev/null \
        || die "helm package failed"
    local tgz="${pkgdir}/rootstock-${chart_version}.tgz"
    [[ -f "$tgz" ]] || die "expected packaged chart ${tgz} not found"
    ok "packaged $(basename "$tgz")"

    # Push over plain HTTP (zot is HTTP on localhost:5000).
    helm push "$tgz" "oci://${registry_host}/${registry_chart_repo}" --plain-http \
        || die "helm push to oci://${registry_host}/${registry_chart_repo} failed"
    ok "pushed chart to oci://${registry_host}/${registry_chart_repo}/rootstock:${chart_version}"

    # Flux keys its OCI chart cache by version; re-pushing the same version without
    # invalidating the HelmChart causes Greenhouse to render the stale artifact.
    # Nudge the HelmRepository and delete the HelmChart to force a fresh pull.
    local hr_names
    hr_names="$(kubectl --context "$admin_context" -n "$demo_org" get helmrepository \
        -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)"
    if [[ -n "$hr_names" ]]; then
        local hr
        for hr in $hr_names; do
            kubectl --context "$admin_context" -n "$demo_org" annotate helmrepository "$hr" \
                "reconcile.fluxcd.io/requestedAt=$(date +%s)" --overwrite >/dev/null 2>&1 || true
        done
    fi
    if kubectl --context "$admin_context" -n "$demo_org" get helmchart "rootstock-${chart_version}" >/dev/null 2>&1; then
        kubectl --context "$admin_context" -n "$demo_org" delete helmchart "rootstock-${chart_version}" >/dev/null 2>&1 || true
        ok "invalidated stale Flux HelmChart cache (rootstock-${chart_version})"
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

# Echo the status (True/False) of a Greenhouse cluster status condition.
cluster_condition() {
    local type="$1"
    kubectl --context "$admin_context" get cluster "$remote_context" -n "$demo_org" \
        -o jsonpath="{.status.statusConditions.conditions[?(@.type==\"${type}\")].status}" \
        2>/dev/null || echo "False"
}

apply_plugindefinition() {
    log "Phase 2: apply PluginDefinition and assert READY=True"

    # Operate on a TEMP COPY — never mutate the committed plugindefinition.yaml.
    # Rewrite helmChart.repository to the in-cluster address, pin versions to chart_version.
    # NOTE: PluginDefinition is namespaced in Greenhouse; it must live in the org namespace
    # (${demo_org}) or the reconciler won't find the org and will never become Ready.
    local pd="${workdir}/plugindefinition.yaml"
    cp "$plugin_def" "$pd"
    yq -i "
      .metadata.namespace = \"${demo_org}\" |
      .spec.version = \"${chart_version}\" |
      .spec.helmChart.repository = \"oci://${registry_in_cluster}/${registry_chart_repo}\" |
      .spec.helmChart.version = \"${chart_version}\"
    " "$pd"
    ok "temp PluginDefinition: namespace=${demo_org}, version=${chart_version}, repository=oci://${registry_in_cluster}/${registry_chart_repo}"

    kubectl --context "$admin_context" -n "$demo_org" apply -f "$pd" \
        || die "kubectl apply of PluginDefinition failed"
    ok "applied PluginDefinition 'rootstock' in ns ${demo_org}"

    if wait_ready plugindefinition rootstock "$demo_org"; then
        ok "PluginDefinition rootstock is READY=True"
    else
        die "PluginDefinition rootstock did not become READY within ${READY_TIMEOUT}s"
    fi
}

# =============================================================================
# OCI image publishing into the local registry (zot).
#
# Greenhouse resolves the HelmRelease image from zot, so the image must live
# there (a `kind load` is not enough). zot only accepts OCI manifests, so we
# push via a docker-container buildx builder on the host network (the plain
# `docker` driver can't push to an insecure registry, and network=host is
# needed to reach localhost:5000).

# Create the shared OCI builder once; safe to call repeatedly.
ensure_oci_builder() {
    if docker buildx inspect "${oci_builder}" >/dev/null 2>&1; then
        return
    fi
    docker buildx create --name "${oci_builder}" \
        --driver docker-container \
        --driver-opt network=host \
        --buildkitd-flags '--allow-insecure-entitlement network.host' \
        >/dev/null 2>&1 \
        || die "failed to create buildx builder ${oci_builder}"
    docker buildx inspect --bootstrap "${oci_builder}" >/dev/null 2>&1 \
        || { docker buildx rm "${oci_builder}" >/dev/null 2>&1 || true; \
             die "failed to bootstrap buildx builder ${oci_builder}"; }
}

# Re-publish a local docker-daemon image to the OCI registry without rebuilding.
#
# The docker-container builder has its own image store and can't read daemon
# images, so we bridge the two with `docker save` (Docker >=25 writes an
# OCI-layout tarball) and feed the extracted layout to the builder as a build
# context — layers are copied, nothing is recompiled. Missing images are pulled
# first, so upstream refs (e.g. curlimages/curl) work too.
#
#   push_local_image_oci <dest_host> <local_image>
push_local_image_oci() {
    local dest_host="$1"
    local local_image="$2"
    local ctx layout

    [[ -n "$local_image" ]] || die "push_local_image_oci: local_image is required"
    if ! docker image inspect "$local_image" >/dev/null 2>&1; then
        log "image ${local_image} not present locally; pulling"
        docker pull "$local_image" >/dev/null 2>&1 \
            || die "image ${local_image} not found locally and could not be pulled; build it first (e.g. 'make docker-build')"
    fi

    log "re-publishing local image ${local_image} as OCI ${dest_host} (no rebuild)"
    ensure_oci_builder

    ctx="$(mktemp -d "${TMPDIR:-/tmp}/ghverify-oci.XXXXXX")"
    layout="${ctx}/layout"
    mkdir -p "$layout"

    # `docker save` streams an OCI-layout tarball; extract it into $layout.
    docker save "$local_image" | tar -x -C "$layout" \
        || { rm -rf "$ctx"; die "failed to export local image ${local_image} via docker save"; }
    [[ -f "${layout}/index.json" ]] \
        || { rm -rf "$ctx"; die "docker save did not produce an OCI layout; Docker >= 25 is required to reuse the local image"; }

    # A throwaway `FROM base` Dockerfile whose base is the extracted OCI layout.
    printf 'FROM base\n' > "${ctx}/Dockerfile"

    docker buildx --builder "${oci_builder}" build \
        --platform "${BUILD_PLATFORM:-linux/arm64}" \
        --provenance=false \
        --build-context "base=oci-layout://${layout}" \
        --output "type=registry,registry.insecure=true,name=${dest_host}" \
        -f "${ctx}/Dockerfile" "$ctx" \
        || { rm -rf "$ctx"; die "OCI push of ${dest_host} (from local image ${local_image}) failed"; }

    rm -rf "$ctx"
    ok "pushed OCI image ${dest_host} (reused local ${local_image})"
}

# =============================================================================
# Phase 3: optional full Plugin E2E (--with-plugin)
# =============================================================================
run_plugin_e2e() {
    log "Phase 3: full Plugin E2E (--with-plugin)"

    # 1. Make the rootstock image available to the remote cluster via the local
    #    registry (zot). Greenhouse resolves the HelmRelease image from the
    #    registry, so a `kind load` into node containerd is not enough -- we push
    #    an OCI image and point the Plugin at registry_in_cluster/rootstock.
    if ! docker image inspect "$ROOTSTOCK_IMAGE" >/dev/null 2>&1; then
        die "image ${ROOTSTOCK_IMAGE} not found locally; build it first (e.g. 'make docker-build' / 'make kind-up')"
    fi
    # Reuse the image already built in the local docker daemon (make docker-build)
    # instead of rebuilding it from the Dockerfile. Push using the HOST-visible
    # address; the Plugin references registry_in_cluster (same zot, but reachable
    # in-cluster).
    push_local_image_oci "${registry_host}/rootstock:${ROOTSTOCK_IMAGE##*:}" "$ROOTSTOCK_IMAGE"
    ok "image available in registry as ${rootstock_registry_image}"

    # Re-publish the test-connection curl image as OCI; dev-env zot can't mirror
    # Docker Hub (docker2s2 disabled), so override testConnection.image.* in the Plugin.
    push_local_image_oci "${registry_host}/${test_conn_image}" "$test_conn_image"
    ok "test-connection image available in registry as ${testconn_registry_image}"

    # Re-publish the SNMP exporter image as OCI; dev-env zot can't mirror quay.io,
    # so override snmpExporter.image.* in the Plugin. Strip the quay.io host so the
    # local path matches snmp_exporter_registry_image.
    push_local_image_oci "${registry_host}/${snmp_exporter_image#*/}" "$snmp_exporter_image"
    ok "snmp-exporter image available in registry as ${snmp_exporter_registry_image}"

    # 2. Wait for Greenhouse to register the remote cluster's nodes as ready.
    #    Freshly created clusters race: nodes go Ready before Greenhouse updates
    #    the cluster status, which would otherwise fail Plugin deployment.
    log "waiting for Greenhouse to recognize cluster nodes as ready"
    local timeout=300 elapsed=0 refreshed=false
    local nodes_ready payload_schedulable
    while true; do
        nodes_ready="$(cluster_condition AllNodesReady)"
        payload_schedulable="$(cluster_condition PayloadSchedulable)"
        if [[ "$nodes_ready" == "True" && "$payload_schedulable" == "True" ]]; then
            echo
            ok "cluster nodes are ready and schedulable in Greenhouse"
            break
        fi
        if (( elapsed >= timeout )); then
            echo
            die "cluster ${remote_context} not ready after ${timeout}s (AllNodesReady=${nodes_ready}, PayloadSchedulable=${payload_schedulable})"
        fi
        # Nudge a status refresh once at the 60s mark.
        if (( elapsed >= 60 )) && [[ "$refreshed" == false ]]; then
            echo; log "forcing cluster status refresh"
            kubectl --context "$admin_context" annotate cluster "$remote_context" -n "$demo_org" \
                greenhouse.sap/force-refresh="$(date +%s)" --overwrite >/dev/null 2>&1 || true
            refreshed=true
        fi
        printf '.'
        sleep 5
        elapsed=$(( elapsed + 5 ))
    done

    # 3. Redfish Secret in the org namespace (Greenhouse resolves valueFrom.secret there).
    local redfish_pw="${REDFISH_PASSWORD:-redfish-test-password}"
    kubectl --context "$admin_context" -n "$demo_org" create secret generic rootstock-redfish \
        --from-literal=redfish_password="$redfish_pw" \
        --dry-run=client -o yaml | kubectl --context "$admin_context" apply -f - \
        || die "failed to create rootstock-redfish secret"
    ok "created Secret rootstock-redfish in ns ${demo_org}"

    # 4. Apply the Plugin on a temp copy. Normalize to the pinned Greenhouse CRD schema
    # (spec.pluginDefinitionRef, drop legacy spec.pluginDefinition/disabled), repoint
    # image.repository/tag at the local registry, and flip pullPolicy to IfNotPresent.
    local plugin="${workdir}/plugin.yaml"
    cp "${greenhouse_test_dir}/plugin.yaml" "$plugin"
    yq -i "
      .metadata.namespace = \"${demo_org}\" |
      .spec.clusterName = \"${remote_context}\" |
      .spec.pluginDefinitionRef = {\"kind\": \"PluginDefinition\", \"name\": \"rootstock\"} |
      del(.spec.pluginDefinition) |
      del(.spec.disabled) |
      (.spec.optionValues[] | select(.name == \"image.pullPolicy\").value) = \"IfNotPresent\" |
      (.spec.optionValues[] | select(.name == \"image.repository\").value) = \"${rootstock_registry_image%:*}\" |
      (.spec.optionValues[] | select(.name == \"image.tag\").value) = \"${rootstock_registry_image##*:}\" |
      .spec.optionValues += [
        {\"name\": \"snmpExporter.enabled\", \"value\": true},
        {\"name\": \"snmpExporter.configFile\", \"value\": \"${SNMP_CONFIG_FILE}\"},
        {\"name\": \"snmpExporter.image.repository\", \"value\": \"${snmp_exporter_registry_image%:*}\"},
        {\"name\": \"snmpExporter.image.tag\", \"value\": \"${snmp_exporter_registry_image##*:}\"},
        {\"name\": \"testConnection.image.repository\", \"value\": \"${testconn_registry_image%:*}\"},
        {\"name\": \"testConnection.image.tag\", \"value\": \"${testconn_registry_image##*:}\"}
      ]
    " "$plugin"
    kubectl --context "$admin_context" apply -f "$plugin" \
        || die "kubectl apply of Plugin failed"
    ok "applied Plugin 'rootstock' (cluster=${remote_context}, ns=${demo_org})"

    if wait_ready plugin rootstock "$demo_org"; then
        ok "Plugin rootstock is READY=True"
    else
        die "Plugin rootstock did not become READY within ${READY_TIMEOUT}s"
    fi

    # 4. Skip probe check (no mockup deployment)
    log "Skipping probe check (no Redfish target configured)"
    ok "Plugin deployed successfully"
}

# =============================================================================
# Phase 4: teardown
# =============================================================================
teardown() {
    local rc=$?
    [[ -n "$workdir" && -d "$workdir" ]] && rm -rf "$workdir"
    # Always remove the shared OCI buildx builder (a host-side resource, not part
    # of the kept cluster state).
    docker buildx rm "${oci_builder}" >/dev/null 2>&1 || true
    return $rc
}

# =============================================================================
# Version resolution
# =============================================================================
# Resolve the chart version (always from Chart.yaml), honouring the
# ROOTSTOCK_IMAGE override. Called once at the start of main() so every phase
# stays in lockstep.
resolve_versions() {
    log "resolving versions"

    chart_version="$(yq -r '.version' "${chart_dir}/Chart.yaml" 2>/dev/null)" \
        || die "failed to read .version from ${chart_dir}/Chart.yaml"
    [[ -n "$chart_version" && "$chart_version" != "null" ]] \
        || die "chart version is empty/null in ${chart_dir}/Chart.yaml"
    ok "chart version ${chart_version} (from Chart.yaml)"

    if [[ -z "$ROOTSTOCK_IMAGE" ]]; then
        # Mirror the Makefile default: IMAGE ?= ghcr.io/cloudoperators/rootstock:dev-$(USER)
        ROOTSTOCK_IMAGE="ghcr.io/cloudoperators/rootstock:dev-${USER}"
        ok "image ${ROOTSTOCK_IMAGE} (Makefile IMAGE default)"
    else
        ok "image ${ROOTSTOCK_IMAGE} (ROOTSTOCK_IMAGE override)"
    fi

    rootstock_registry_image="${registry_in_cluster}/rootstock:${ROOTSTOCK_IMAGE##*:}"
    ok "registry image ${rootstock_registry_image} (derived)"

    # Test-hook image re-published under the in-cluster registry
    # (curlimages/curl:8.11.1 -> ${registry_in_cluster}/curlimages/curl:8.11.1).
    testconn_registry_image="${registry_in_cluster}/${test_conn_image}"
    ok "test-connection image ${testconn_registry_image} (derived)"

    # SNMP exporter image re-published under the in-cluster registry (strip the
    # quay.io host: quay.io/prometheus/snmp-exporter:v0.26.0 ->
    # ${registry_in_cluster}/prometheus/snmp-exporter:v0.26.0).
    snmp_exporter_registry_image="${registry_in_cluster}/${snmp_exporter_image#*/}"
    ok "snmp-exporter image ${snmp_exporter_registry_image} (derived)"
}

main "$@"
