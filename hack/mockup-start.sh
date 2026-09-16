#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PDU_MOCKUP_PATH="${PDU_MOCKUP_PATH:-${SCRIPT_DIR}/../testdata/pdu-mockup}"
MOCKUP_PORT="${MOCKUP_PORT:-8000}"
MOCKUP_CONTAINER="${MOCKUP_CONTAINER:-redfish-mockup}"

echo "Starting Redfish mockup server on port ${MOCKUP_PORT}..."
docker run -d --rm \
    --name "${MOCKUP_CONTAINER}" \
    -p "${MOCKUP_PORT}":8000 \
    -v "$(realpath "${PDU_MOCKUP_PATH}")":/usr/src/app/pdu-mockup \
    dmtf/redfish-mockup-server:latest -D pdu-mockup

echo "Waiting for mockup server to be ready..."
for i in $(seq 1 30); do
    curl -sf "http://localhost:${MOCKUP_PORT}/redfish/v1/PowerEquipment" > /dev/null 2>&1 \
        && echo "Mockup server ready." && exit 0
    sleep 1
done

echo "ERROR: mockup server did not start within 30s"
exit 1
