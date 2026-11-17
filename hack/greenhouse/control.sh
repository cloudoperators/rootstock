#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# control.sh
# Bootstrap a local Greenhouse-in-kind environment.
#
# Clones cloudoperators/greenhouse at a pinned ref and runs the Greenhouse make
# setup targets (default: setup-manager setup-demo) that bring up admin+remote
# kind clusters, the zot registry, and the demo Organization.
#
# Run standalone to provision a fresh Greenhouse dev environment.
# Also sourced by start_plugin.sh to reuse its functions without
# re-running main().
set -euo pipefail

# shellcheck source=../_helper.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../_helper.sh"

# --- Configuration (all overridable) -----------------------------------------
GREENHOUSE_REF="${GREENHOUSE_REF:-v0.18.0}"
DEMO_ORG="demo"

greenhouse_repo="https://github.com/cloudoperators/greenhouse"

admin_cluster="greenhouse-admin"
remote_cluster="greenhouse-remote"

admin_context="kind-${admin_cluster}"
remote_context="kind-${remote_cluster}"

rootstock_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workdir="${rootstock_root}/tmp/greenhouse/setup"
greenhouse_dir="${workdir}/greenhouse"

greenhouse_setup_targets="setup-manager setup-demo"

# =============================================================================
# main
# =============================================================================
main() {
    local cmd="${1:-}"
    [[ -z "$cmd" ]] && { usage; die "command required: create or delete"; }
    shift
    case "$cmd" in
        create)    cmd_create "$@" ;;
        delete)    cmd_delete "$@" ;;
        -h|--help) usage; exit 0 ;;
        *)         usage; die "unknown command: ${cmd}" ;;
    esac
}

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [OPTIONS]

Commands:
  create    Bootstrap a local Greenhouse-in-kind environment (clone + make setup).
  delete    Delete the kind clusters, registry container, and workdir.

create options:
  --ref REF           Greenhouse git tag/commit to pin
                      (env: GREENHOUSE_REF, default: ${GREENHOUSE_REF})
  -h, --help          Show this help.

delete options:
  -h, --help          Show this help.
EOF
}

cmd_create() {
    local ref="${GREENHOUSE_REF}"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ref)     ref="$2"; shift 2 ;;
            -h|--help) usage; exit 0 ;;
            *)         usage; die "unknown argument: $1" ;;
        esac
    done

    GREENHOUSE_REF="$ref"

    preflight_tools
    check_registry_port
    fetch_greenhouse
    bring_up_greenhouse
}

cmd_delete() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h|--help)        usage; exit 0 ;;
            *)                usage; die "unknown argument: $1" ;;
        esac
    done

    log_info "teardown: removing kind clusters, registry, and workdir"
    kind delete cluster --name "$admin_cluster" >/dev/null 2>&1 || true
    kind delete cluster --name "$remote_cluster" >/dev/null 2>&1 || true
    docker rm -f registry >/dev/null 2>&1 || true
    [[ -d "$workdir" ]] && rm -rf "$workdir"
    log_info "removed clusters, registry, and workdir"
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
    log_info "all tools present and docker daemon reachable"
}

# Ensure host port 5000 is free for Greenhouse's local registry. Stale `registry`
# containers from a previous run are removed automatically; any other listener
# (e.g. macOS AirPlay Receiver) must be freed manually — Greenhouse hard-codes port 5000.
check_registry_port() {
    local port=5000

    # A leftover `registry` container is ours to remove.
    if docker inspect registry >/dev/null 2>&1; then
        log_warn "removing stale 'registry' container from a previous run"
        docker rm -f registry >/dev/null 2>&1 || true
    fi

    local holder=""
    if command -v lsof >/dev/null 2>&1; then
        # lsof exits non-zero when nothing is listening; `|| true` prevents set -e from aborting.
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
    log_info "Phase 1: fetch Greenhouse @ ${GREENHOUSE_REF}"
    mkdir -p "$workdir"
    if [[ -d "${greenhouse_dir}/.git" ]]; then
        local have
        have="$(git -C "$greenhouse_dir" describe --tags --always 2>/dev/null || echo unknown)"
        if [[ "$have" == "$GREENHOUSE_REF" ]]; then
            log_info "existing clone already at ${GREENHOUSE_REF}, reusing"
            return
        fi
        log_warn "existing clone is at '${have}', re-cloning at ${GREENHOUSE_REF}"
        rm -rf "$greenhouse_dir"
    fi
    _exec git clone --depth 1 --branch "$GREENHOUSE_REF" "$greenhouse_repo" "$greenhouse_dir"
    log_info "cloned Greenhouse into ${greenhouse_dir}"
}

# =============================================================================
# Phase 2: bring up Greenhouse
# =============================================================================
bring_up_greenhouse() {
    log_info "Phase 2: bootstrap local Greenhouse (make setup) -- this is slow and can be flaky on first run"
    # Run only setup-manager + setup-demo by default, skipping setup-dashboard:
    # the dashboard step is flaky locally and not needed for PluginDefinition verification.
    # Override greenhouse_setup_targets for the full upstream flow.
    local setup_targets="${greenhouse_setup_targets:-setup-manager setup-demo}"
    log_info "running: make ${setup_targets}"
    (
        cd "$greenhouse_dir"
        # shellcheck disable=SC2086 # intentional word-splitting of target list
        make ${setup_targets}
    ) || die "Greenhouse 'make ${setup_targets}' failed (see output above)"

    # setup-demo switches the current context, but be explicit.
    _exec "kubectl config get-contexts '$admin_context' >/dev/null 2>&1"
    log_info "Greenhouse is up (admin=${admin_context}, remote=${remote_context}, org ns=${DEMO_ORG})"

    # Sanity: PluginDefinition CRD must exist or apply will fail cryptically.
    _exec "kubectl --context '$admin_context' get crd plugindefinitions.greenhouse.sap >/dev/null 2>&1"
    log_info "PluginDefinition CRD present"
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
