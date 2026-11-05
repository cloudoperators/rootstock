#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0

# Stop Prometheus monitoring stack deployed via Greenhouse Plugin

set -euo pipefail

green='\033[1;32m'
yellow='\033[1;33m'
reset='\033[0m'

log() { echo -e "${green}[+]${reset} $*"; }
warn() { echo -e "${yellow}[!]${reset} $*"; }

# Configuration
ADMIN_CONTEXT="${ADMIN_CONTEXT:-kind-greenhouse-admin}"
DEMO_ORG="${DEMO_ORG:-demo}"

# Check prerequisites
command -v kubectl >/dev/null 2>&1 || {
    warn "kubectl not found in PATH"
    exit 1
}

# Check if admin context exists
if ! kubectl config get-contexts "$ADMIN_CONTEXT" >/dev/null 2>&1; then
    warn "Admin context '${ADMIN_CONTEXT}' not found"
    exit 1
fi

# Stop port-forward processes for Prometheus
log "stopping Prometheus port-forward processes..."
pkill -f "port-forward.*prometheus.*9090" 2>/dev/null || true
pkill -f "port-forward.*monitoring" 2>/dev/null || true

# Delete the Plugin CR (this will trigger cleanup of all resources)
log "deleting Prometheus Plugin from admin cluster..."
if kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" get plugin prometheus-monitoring >/dev/null 2>&1; then
    kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" delete plugin prometheus-monitoring --wait=false >/dev/null 2>&1
    log "Prometheus Plugin deletion initiated"
else
    warn "Prometheus Plugin not found"
fi

log "cleanup complete"
echo
echo "Note: The Plugin deletion is asynchronous. Resources on the remote cluster"
echo "will be removed by Greenhouse. To check status:"
echo "  kubectl --context ${ADMIN_CONTEXT} -n ${DEMO_ORG} get plugin prometheus-monitoring"
