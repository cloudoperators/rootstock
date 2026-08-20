# Design

## Problem

Data centers run hundreds of PDUs from multiple vendors. There is no standard way to collect energy and carbon telemetry from them at scale and surface it in observability tooling.

## Goals

- Collect energy metrics (power consumption in Watts, energy usage in kWh, power factor) from rack PDUs
- Scale to 100+ racks using the multi-target pattern (one pod, many targets)
- Support multiple vendors via the Redfish protocol; initial implementation targets Panduit
- Store metrics in Prometheus; visualize in Perses
- Ship a production-ready Helm chart with ServiceMonitor, RBAC, and health checks
- Expose self-monitoring metrics: scrape duration, error counts, target health

## Non-goals

- GPU or server-level telemetry (out of scope for v1)
- Vendor-specific protocols other than Redfish
- Direct database writes; Prometheus pull model only

## Requirements

### Functional

| Requirement | Detail |
|---|---|
| Multi-target scraping | One rootstock instance handles all PDU targets |
| Redfish support | Queries `/redfish/v1/`, `/redfish/v1/PowerEquipment`, `/redfish/v1/PowerEquipment/RackPDUs` |
| Prometheus format | Metrics returned as `text/plain; version=0.0.4` |
| Health probes | `/health` (liveness) and `/ready` (readiness) endpoints |
| Graceful shutdown | In-flight requests complete before process exits |

### Non-functional

| Requirement | Detail |
|---|---|
| Scale | 100+ racks |
| Scrape frequency | Not too frequent; configurable interval |
| Resilience | Configurable timeouts and retries per PDU |
| Observability | Self-monitoring metrics exposed on `/metrics` |

## Security

### SSRF protection

Rootstock validates every `target` parameter before connecting:

| Check | Rule |
|---|---|
| Hostname whitelist | Only hosts in the configured allowlist are contacted |
| IP blocking | Raw IP addresses are rejected; `169.254.169.254` (metadata API) is blocked |
| Port restriction | Only standard HTTPS port allowed |
| Domain suffix validation | Configurable suffix allowlist (e.g., `.example.com`) |

### Credential security

| Rule | Implementation |
|---|---|
| Credentials never logged | Redfish username/password stripped from all log output |
| Kubernetes Secrets | Credentials mounted from K8s Secret, not environment variables |
| Vault support | Optional integration for secret rotation |

## Driver

EU sustainability legislation requires data centers to report energy consumption. Rootstock provides the collection layer that feeds dashboards and carbon accounting tools in Greenhouse.

## See also

- [Architecture](./architecture) — component diagram and data flow
- [Getting Started](../getting-started) — deploy rootstock
- [API Reference](../reference/api) — HTTP endpoints
