# Prometheus Integration

## Table of Contents

- [1. Overview](#1-overview)
  - [Architecture](#architecture)
  - [Design Principles](#design-principles)
- [2. Components](#2-components)
  - [2.1 Prometheus Collector](#21-prometheus-collector)
  - [2.2 Metric Definitions](#22-metric-definitions)
  - [2.3 Redfish Client Integration](#23-redfish-client-integration)
  - [2.4 HTTP Server](#24-http-server)
- [3. Usage Example](#3-usage-example)
  - [Prometheus Configuration](#prometheus-configuration)
- [4. Future Enhancements](#4-future-enhancements)
  - [Dynamic Target Probing](#dynamic-target-probing)
  - [Performance Optimizations](#performance-optimizations)

---

## 1. Overview

Rootstock will provide multiple functionalities regarding data center power
reporting in the future. One of these functionalities is acting as a
**Prometheus exporter** that probes Power Distribution Units (PDUs) to expose
power and energy metrics.

### Architecture

The integration follows an **active probing model** where Prometheus triggers
on-demand queries to PDU devices:

```
┌─────────────┐         ┌──────────────┐         ┌───────────────┐         ┌─────────┐
│  Prometheus │──scrape→│   Rootstock  │──probe─→│ Redfish Client│──HTTP──→│   PDU   │
│   Server    │←────────│  /metrics    │←────────│   (on-demand) │←────────│  Device │
└─────────────┘         └──────────────┘         └───────────────┘         └─────────┘
                            ↓
                      Custom Collector
                      (prometheus.Collector)
```

### Design Principles

- **Active Probing**: Metrics are fetched from PDU Prometheus scrapes (for the
  POC we won't implement background collection)
- **Custom Collector**: Implements `prometheus.Collector` interface for dynamic,
  on-demand metric generation
- **Redfish Integration**: Uses Redfish session-based authentication to query
  PDU power metrics
- **OpenTelemetry Future**: While this implementation focuses on Prometheus
  native exposition, OpenTelemetry Collector integration is planned for broader
  telemetry vendor support

## 2. Components

### 2.1 Prometheus Collector

**Location**: `internal/collectors/prometheus/collector.go`

The collector implements the `prometheus.Collector` interface with two required
methods:

#### `Describe(ch chan<- *prometheus.Desc)`

Registers metric descriptors with Prometheus. Called once during collector
registration.

#### `Collect(ch chan<- prometheus.Metric)`

Probes the PDU via Redfish API and sends fresh metrics to Prometheus. Called on
**every scrape**.

**Probing Flow**:

```
1. Prometheus scrapes the /metrics HTTP endpoint
2. Prometheus registry triggers collector.Collect()
3. Mutex lock acquired (ensures thread-safe probing)
4. redfishClient.GetPDUMetrics(ctx, pduID) executes HTTP request to PDU - eventually cache will be added to the probing flow
   (Additional Redfish client calls will be added in the future)
5. Redfish JSON response parsed into structured data - eventually saving cache will be added to the flow
6. Prometheus metrics created with NewConstMetric() with error handling
7. Metrics sent to Prometheus via channel
8. Mutex released
9. Prometheus formats response in exposition format
```

**Key Characteristics**:

- **Stateless**: No metric caching between scrapes
- **Synchronous**: Scrape duration includes PDU query time
- **Thread-safe**: Mutex prevents concurrent probes to same PDU

### 2.2 Metric Definitions

The collector exposes the following **gauge metrics** for power monitoring (all
labeled with `pdu_id`). This list will grow as additional metrics are
implemented:

| Metric Name        | Type  | Description                     | Unit                        | Labels   |
| ------------------ | ----- | ------------------------------- | --------------------------- | -------- |
| `pdu_power_watts`  | Gauge | Real power consumption          | Watts (W)                   | `pdu_id` |
| `pdu_apparent_va`  | Gauge | Apparent power                  | Volt-Amperes (VA)           | `pdu_id` |
| `pdu_reactive_var` | Gauge | Reactive power                  | Volt-Amperes Reactive (VAR) | `pdu_id` |
| `pdu_power_factor` | Gauge | Power factor (efficiency ratio) | Dimensionless (0-1)         | `pdu_id` |

### 2.3 Redfish Client Integration

**Location**: `internal/redfish/client.go`

The Redfish client handles:

- Session management (login/logout) with X-Auth-Token
- HTTP requests to Redfish API endpoints
- JSON response parsing
- TLS configuration (currently insecure skip verify for development)

### 2.4 HTTP Server

**Location**: `cmd/server/main.go`

The HTTP server exposes standard Prometheus endpoints:

```go
r.Handle(metricsEndpoint, promhttp.Handler())
```

**Available Endpoints**:

- `GET /metrics` - Prometheus exposition format (includes Go runtime metrics +
  PDU metrics)
- `GET /healthz` - Liveness probe (returns 200 OK)
- `GET /ready` - Readiness probe
- `GET /probe` - **Not yet implemented** - Future: Dynamic target probing

Additional endpoints will be added as new features are implemented.

## 3. Usage Example

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: "rootstock-pdu"
    scrape_interval: 5m
    scrape_timeout: 10s
    static_configs:
      - targets: ["localhost:8080"]
        labels:
          environment: "production"
          datacenter: "dc1"
```

## 4. Future Enhancements

### Dynamic Target Probing

Implement `/probe?target=<pdu-id>`:

```yaml
scrape_configs:
  - job_name: "pdu-probe"
    metrics_path: /probe
    static_configs:
      - targets: [pdu-01, pdu-02, pdu-03]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - target_label: __address__
        replacement: localhost:8080
```

### Performance Optimizations

- **Caching**: Which is part of the design, not in the POC
- **Concurrent Collection**: Worker pool for multi-PDU deployments
- **Context Timeouts**: Proper deadline management
