#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0

# Start Prometheus to scrape Rootstock metrics from the Greenhouse test environment.
# This script deploys Prometheus using the Greenhouse PluginDefinition and Plugin CR,
# creating a full kube-prometheus-stack deployment with custom scrape configs for rootstock.

set -euo pipefail

# Colors
green='\033[1;32m'
yellow='\033[1;33m'
red='\033[1;31m'
reset='\033[0m'

log() { echo -e "${green}[+]${reset} $*"; }
warn() { echo -e "${yellow}[!]${reset} $*"; }
die() { echo -e "${red}[✗]${reset} $*" >&2; exit 1; }

# Configuration
ROOTSTOCK_DIR="${ROOTSTOCK_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
PROMETHEUS_PORT="${PROMETHEUS_PORT:-9090}"

# Kubernetes contexts
ADMIN_CONTEXT="${ADMIN_CONTEXT:-kind-greenhouse-admin}"
REMOTE_CONTEXT="${REMOTE_CONTEXT:-kind-greenhouse-remote}"
ROOTSTOCK_NAMESPACE="${ROOTSTOCK_NAMESPACE:-rootstock}"
PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-monitoring}"
DEMO_ORG="${DEMO_ORG:-demo}"

# Paths
PLUGINDEF_FILE="${ROOTSTOCK_DIR}/k8s_configuration/prometheus-plugindefinition.yaml"
PLUGIN_FILE="${ROOTSTOCK_DIR}/k8s_configuration/greenhouse-test/prometheus-plugin.yaml"

# Timeouts
READY_TIMEOUT="${READY_TIMEOUT:-300}"
POLL_INTERVAL="${POLL_INTERVAL:-5}"

# Check prerequisites
command -v kubectl >/dev/null 2>&1 || die "kubectl not found in PATH"
command -v helm >/dev/null 2>&1 || die "helm not found in PATH"

# Check if contexts exist
log "checking Kubernetes contexts..."
if ! kubectl config get-contexts "$ADMIN_CONTEXT" >/dev/null 2>&1; then
    die "Admin context '${ADMIN_CONTEXT}' not found. Run 'make greenhouse-create' first."
fi
if ! kubectl config get-contexts "$REMOTE_CONTEXT" >/dev/null 2>&1; then
    die "Remote context '${REMOTE_CONTEXT}' not found. Run 'make greenhouse-create' first."
fi

# Check if Rootstock is running
log "checking if Rootstock is running in ${REMOTE_CONTEXT}..."
if ! kubectl --context "$REMOTE_CONTEXT" get deployment rootstock -n "$ROOTSTOCK_NAMESPACE" >/dev/null 2>&1; then
    die "Rootstock deployment not found in cluster ${REMOTE_CONTEXT}, namespace ${ROOTSTOCK_NAMESPACE}"
fi

if ! kubectl --context "$REMOTE_CONTEXT" wait --for=condition=Ready pod -l app.kubernetes.io/name=rootstock -n "$ROOTSTOCK_NAMESPACE" --timeout=10s >/dev/null 2>&1; then
    die "Rootstock pod is not ready"
fi

log "Rootstock is running and ready"

# Check if required files exist
[[ -f "$PLUGINDEF_FILE" ]] || die "PluginDefinition not found at ${PLUGINDEF_FILE}"
[[ -f "$PLUGIN_FILE" ]] || die "Plugin CR not found at ${PLUGIN_FILE}"

# Helper function to wait for resource to be ready
wait_ready() {
    local kind="$1" name="$2" namespace="$3" context="$4"
    local deadline=$(( $(date +%s) + READY_TIMEOUT ))
    local status=""
    local last_status=""
    
    log "waiting for ${kind}/${name} to become ready (timeout: ${READY_TIMEOUT}s)..."
    while (( $(date +%s) < deadline )); do
        status="$(kubectl --context "$context" -n "$namespace" get "$kind" "$name" \
            -o jsonpath='{.status.statusConditions.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)"
        
        # Show status updates
        if [[ "$status" != "$last_status" && -n "$status" ]]; then
            log "${kind}/${name} status: ${status}"
            last_status="$status"
        fi
        
        if [[ "$status" == "True" ]]; then
            log "${kind}/${name} is ready"
            return 0
        fi
        sleep "$POLL_INTERVAL"
    done
    
    warn "${kind}/${name} did not become ready within ${READY_TIMEOUT}s (last status: ${status:-unknown})"
    kubectl --context "$context" -n "$namespace" get "$kind" "$name" -o yaml | grep -A 20 'status:' || true
    return 1
}

# Helper function to wait for HelmRelease
wait_helmrelease() {
    local name="$1" namespace="$2" context="$3"
    local deadline=$(( $(date +%s) + READY_TIMEOUT ))
    local status=""
    
    log "waiting for HelmRelease/${name} to be deployed..."
    while (( $(date +%s) < deadline )); do
        status="$(kubectl --context "$context" -n "$namespace" get helmrelease "$name" \
            -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)"
        
        if [[ "$status" == "True" ]]; then
            log "HelmRelease/${name} deployed successfully"
            return 0
        fi
        
        # Check if it's still installing
        local hr_status="$(kubectl --context "$context" -n "$namespace" get helmrelease "$name" \
            -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}' 2>/dev/null || true)"
        
        if [[ -n "$hr_status" && "$hr_status" != *"Running"* ]]; then
            log "HelmRelease status: $hr_status"
        fi
        
        sleep "$POLL_INTERVAL"
    done
    
    warn "HelmRelease/${name} deployment timed out after ${READY_TIMEOUT}s"
    kubectl --context "$context" -n "$namespace" describe helmrelease "$name" || true
    return 1
}

# Create monitoring namespace on remote cluster
log "creating namespace ${PROMETHEUS_NAMESPACE} on remote cluster..."
kubectl --context "$REMOTE_CONTEXT" create namespace "$PROMETHEUS_NAMESPACE" --dry-run=client -o yaml | \
    kubectl --context "$REMOTE_CONTEXT" apply -f - >/dev/null

# Apply PluginDefinition to admin cluster
log "applying Prometheus PluginDefinition to admin cluster..."
kubectl --context "$ADMIN_CONTEXT" apply -f "$PLUGINDEF_FILE" >/dev/null

# Wait for PluginDefinition to be ready
wait_ready "PluginDefinition" "prometheus-monitoring" "$DEMO_ORG" "$ADMIN_CONTEXT" || \
    die "PluginDefinition did not become ready"

# Apply Plugin CR to admin cluster
log "applying Prometheus Plugin to admin cluster..."
kubectl --context "$ADMIN_CONTEXT" apply -f "$PLUGIN_FILE" 2>&1 | grep -v "should have a support-group" || true

# Wait for HelmRelease to be created and deployed
log "waiting for HelmRelease to be created..."
for i in {1..30}; do
    if kubectl --context "$ADMIN_CONTEXT" -n "$DEMO_ORG" get helmrelease prometheus-monitoring >/dev/null 2>&1; then
        log "HelmRelease created"
        break
    fi
    sleep 2
done

# Wait for HelmRelease to be ready
wait_helmrelease "prometheus-monitoring" "$DEMO_ORG" "$ADMIN_CONTEXT" || {
    warn "HelmRelease not ready, but continuing to check pods..."
}

# Wait for Prometheus pods to be ready on remote cluster
log "waiting for Prometheus pods to be ready on remote cluster..."
kubectl --context "$REMOTE_CONTEXT" -n "$PROMETHEUS_NAMESPACE" wait --for=condition=Ready \
    pod -l app.kubernetes.io/name=prometheus --timeout="${READY_TIMEOUT}s" 2>&1 || {
    warn "Prometheus pods not ready yet, checking status..."
    kubectl --context "$REMOTE_CONTEXT" -n "$PROMETHEUS_NAMESPACE" get pods
}

log "Prometheus stack deployed successfully!"

# Start port-forward to Prometheus UI
log "starting port-forward to Prometheus UI..."
kubectl --context "$REMOTE_CONTEXT" -n "$PROMETHEUS_NAMESPACE" \
    port-forward svc/kube-prometheus-stack-prometheus "${PROMETHEUS_PORT}:9090" \
    >/dev/null 2>&1 &
PROM_PF_PID=$!

# Give port-forward time to establish
sleep 3

if ! kill -0 "$PROM_PF_PID" 2>/dev/null; then
    warn "port-forward may have failed, checking service..."
    kubectl --context "$REMOTE_CONTEXT" -n "$PROMETHEUS_NAMESPACE" get svc | grep prometheus
    die "port-forward to Prometheus failed to start"
fi

log "Prometheus UI port-forward established (PID: ${PROM_PF_PID})"

# Verify Prometheus is accessible
sleep 2
if command -v curl >/dev/null 2>&1; then
    if curl -sf "http://localhost:${PROMETHEUS_PORT}/-/ready" >/dev/null 2>&1; then
        log "Prometheus is ready and accessible!"
    else
        warn "Prometheus may not be fully ready yet"
    fi
fi

echo
echo -e "${green}✓ Success!${reset}"
echo
echo "Prometheus Stack Information:"
echo "  Admin Cluster: ${ADMIN_CONTEXT}"
echo "  Remote Cluster: ${REMOTE_CONTEXT}"
echo "  Namespace: ${PROMETHEUS_NAMESPACE}"
echo "  Prometheus UI: http://localhost:${PROMETHEUS_PORT}"
echo "  Port-forward PID: ${PROM_PF_PID}"
echo
echo "Configured Scrape Jobs for Rootstock:"
echo "  - rootstock-probe: Multi-target probe pattern for Redfish endpoints"
echo "  - rootstock-metrics: Internal metrics from rootstock exporter"
echo
echo "Useful URLs:"
echo "  Prometheus Targets: http://localhost:${PROMETHEUS_PORT}/targets"
echo "  Prometheus Graphs: http://localhost:${PROMETHEUS_PORT}/graph"
echo
echo "To view resources:"
echo "  kubectl --context ${REMOTE_CONTEXT} -n ${PROMETHEUS_NAMESPACE} get pods"
echo "  kubectl --context ${ADMIN_CONTEXT} -n ${DEMO_ORG} get plugin prometheus-monitoring"
echo "  kubectl --context ${ADMIN_CONTEXT} -n ${DEMO_ORG} get helmrelease prometheus-monitoring"
echo
echo "To stop Prometheus (recommended):"
echo "  hack/greenhouse/stop_prometheus.sh"
echo
echo "Or manually:"
echo "  kill ${PROM_PF_PID}  # Stop port-forward"
echo "  kubectl --context ${ADMIN_CONTEXT} -n ${DEMO_ORG} delete plugin prometheus-monitoring  # Uninstall"
echo

# Keep port-forward running
wait "$PROM_PF_PID" 2>/dev/null || true
