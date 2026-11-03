#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# setup_greenhouse.sh
# Bootstrap a local Greenhouse-in-kind environment.
#
# Clones cloudoperators/greenhouse at a pinned ref and runs the Greenhouse make
# setup targets (default: setup-manager setup-demo) that bring up admin+remote
# kind clusters, the zot registry, and the demo Organization.
#
# Run standalone to provision a fresh Greenhouse dev environment.
# Also sourced by verify-plugindefinition.sh to reuse its functions without
# re-running main().
set -euo pipefail

# --- Configuration (all overridable) -----------------------------------------
GREENHOUSE_REF="${GREENHOUSE_REF:-v0.17.2}"
DEMO_ORG="${DEMO_ORG:-demo}"

greenhouse_repo="https://github.com/cloudoperators/greenhouse"

admin_cluster="greenhouse-admin"
remote_cluster="greenhouse-remote"

admin_context="kind-${admin_cluster}"
remote_context="kind-${remote_cluster}"

rootstock_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workdir="${rootstock_root}/tmp/greenhouse/setup"
greenhouse_dir="${workdir}/greenhouse"

greenhouse_setup_targets="setup-manager setup-demo"

# --- Logging helpers ----------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  OK\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  !!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# =============================================================================
# main
# =============================================================================
main() {
    local cmd="${1:-}"
    [[ -z "$cmd" ]] && { usage; die "command required: setup or teardown"; }
    shift
    case "$cmd" in
        setup)     cmd_setup "$@" ;;
        teardown)  cmd_teardown "$@" ;;
        -h|--help) usage; exit 0 ;;
        *)         usage; die "unknown command: ${cmd}" ;;
    esac
}

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [OPTIONS]

Commands:
  setup     Bootstrap a local Greenhouse-in-kind environment (clone + make setup).
  teardown  Delete the kind clusters, registry container, and workdir.

setup options:
  --ref REF           Greenhouse git tag/commit to pin
                      (env: GREENHOUSE_REF, default: ${GREENHOUSE_REF})
  --org NAME          Greenhouse demo Organization namespace
                      (env: DEMO_ORG, default: ${DEMO_ORG})
  -h, --help          Show this help.

teardown options:
  -h, --help          Show this help.
EOF
}

cmd_setup() {
    local ref="${GREENHOUSE_REF}"
    local org="${DEMO_ORG}"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ref)     ref="$2"; shift 2 ;;
            --org)     org="$2"; shift 2 ;;
            -h|--help) usage; exit 0 ;;
            *)         usage; die "unknown argument: $1" ;;
        esac
    done

    GREENHOUSE_REF="$ref"
    DEMO_ORG="$org"

    preflight_tools
    check_registry_port
    fetch_greenhouse
    bring_up_greenhouse
}

cmd_teardown() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h|--help)        usage; exit 0 ;;
            *)                usage; die "unknown argument: $1" ;;
        esac
    done

    log "teardown: removing kind clusters, registry, and workdir"
    kind delete cluster --name "$admin_cluster" >/dev/null 2>&1 || true
    kind delete cluster --name "$remote_cluster" >/dev/null 2>&1 || true
    docker rm -f registry >/dev/null 2>&1 || true
    [[ -d "$workdir" ]] && rm -rf "$workdir"
    ok "removed clusters, registry, and workdir"
}

# Minimal tool check for standalone use.
preflight_tools() {
    local missing=()
    local t
    for t in docker kind kubectl git make; do
        command -v "$t" >/dev/null 2>&1 || missing+=("$t")
    done
    ((${#missing[@]})) && die "missing required tools: ${missing[*]}"
    docker info >/dev/null 2>&1 || die "docker daemon is not reachable (is Docker running?)"
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
    mkdir -p "$workdir"
    if [[ -d "${greenhouse_dir}/.git" ]]; then
        local have
        have="$(git -C "$greenhouse_dir" describe --tags --always 2>/dev/null || echo unknown)"
        if [[ "$have" == "$GREENHOUSE_REF" ]]; then
            ok "existing clone already at ${GREENHOUSE_REF}, reusing"
            return
        fi
        warn "existing clone is at '${have}', re-cloning at ${GREENHOUSE_REF}"
        rm -rf "$greenhouse_dir"
    fi
    git clone --depth 1 --branch "$GREENHOUSE_REF" "$greenhouse_repo" "$greenhouse_dir" \
        || die "git clone of ${greenhouse_repo}@${GREENHOUSE_REF} failed"
    ok "cloned Greenhouse into ${greenhouse_dir}"
}

# =============================================================================
# Phase 2: bring up Greenhouse
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
    # Override greenhouse_setup_targets to change this (e.g. "setup" for the full
    # upstream flow including the dashboard).
    local setup_targets="${greenhouse_setup_targets:-setup-manager setup-demo}"
    log "running: make ${setup_targets}"
    (
        cd "$greenhouse_dir"
        # shellcheck disable=SC2086 # intentional word-splitting of target list
        make ${setup_targets}
    ) || die "Greenhouse 'make ${setup_targets}' failed (see output above)"

    # Export admin kubeconfig context for subsequent kubectl calls. setup-demo
    # already switched the current context, but be explicit/robust.
    kubectl config get-contexts "$admin_context" >/dev/null 2>&1 \
        || die "expected kube-context ${admin_context} not found after setup"
    ok "Greenhouse is up (admin=${admin_context}, remote=${remote_context}, org ns=${DEMO_ORG})"

    # Sanity: the PluginDefinition CRD must exist, else apply will fail cryptically.
    kubectl --context "$admin_context" get crd plugindefinitions.greenhouse.sap >/dev/null 2>&1 \
        || die "PluginDefinition CRD not present on admin cluster -- setup incomplete"
    ok "PluginDefinition CRD present"
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
