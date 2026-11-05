#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0

# Stop Prometheus container and port-forward for Rootstock monitoring

set -euo pipefail

green='\033[1;32m'
reset='\033[0m'

log() { echo -e "${green}[+]${reset} $*"; }

CONTAINER_NAME="prometheus-rootstock"

# Stop and remove Prometheus container
if docker ps -a --format '{​{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    log "stopping Prometheus container..."
    docker stop "$CONTAINER_NAME" 2>/dev/null || true
    docker rm "$CONTAINER_NAME" 2>/dev/null || true
    log "Prometheus container removed"
else
    log "Prometheus container not running"
fi

# Stop port-forward processes
log "stopping port-forward processes..."
pkill -f "port-forward.*rootstock" 2>/dev/null || true

log "cleanup complete"
