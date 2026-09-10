#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

MOCKUP_CONTAINER="${MOCKUP_CONTAINER:-redfish-mockup}"

docker stop "${MOCKUP_CONTAINER}" 2>/dev/null || true
