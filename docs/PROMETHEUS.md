# Prometheus Integration

## 1. Overview

Rootstock will provide multiple functionalities for data center power management in the future. One of these functionalities is acting as a **Prometheus exporter** that probes Power Distribution Units (PDUs) to expose power and energy metrics. Unlike passive exporters that continuously collect metrics in the background, Rootstock actively queries PDUs via the Redfish API only when Prometheus scrapes the metrics endpoint.

### Architecture

The integration follows an **active probing model** where Prometheus triggers on-demand queries to PDU devices:

```
┌─────────────┐         ┌──────────────┐         ┌───────────────┐         ┌─────────┐
│  Prometheus │──scrape→│   Rootstock  │──probe─→│ Redfish Client│──HTTP──→│   PDU   │
│   Server    │←────────│  /metrics    │←───────│   (on-demand) │←────────│  Device │
└─────────────┘         └──────────────┘         └───────────────┘         └─────────┘
                            ↓
                      Custom Collector
                      (prometheus.Collector)
```

### Design Principles

- **Active Probing**: Metrics are fetched from PDU **only when** Prometheus scrapes (no background collection)
- **Custom Collector**: Implements `prometheus.Collector` interface for dynamic, on-demand metric generation
- **Redfish Integration**: Uses Redfish session-based authentication to query PDU power metrics
- **OpenTelemetry Future**: While this implementation focuses on Prometheus native exposition, OpenTelemetry Collector integration is planned for broader telemetry vendor support

## 2. Components

### 2.1 Prometheus Collector

**Location**: `internal/collectors/prometheus/collector.go`

The collector implements the `prometheus.Collector` interface with two required methods:

#### `Describe(ch chan<- *prometheus.Desc)`
Registers metric descriptors with Prometheus. Called once during collector registration.

#### `Collect(ch chan<- prometheus.Metric)`
Probes the PDU via Redfish API and sends fresh metrics to Prometheus. Called on **every scrape**.

**Probing Flow**:
```
1. Prometheus scrapes the /metrics HTTP endpoint
2. Prometheus registry triggers collector.Collect()
3. Mutex lock acquired (ensures thread-safe probing)
4. redfishClient.GetPDUMetrics(ctx, pduID) executes HTTP request to PDU
   (Additional Redfish client calls will be added in the future)
5. Redfish JSON response parsed into structured data
6. Prometheus metrics created with MustNewConstMetric()
7. Metrics sent to Prometheus via channel
8. Mutex released
9. Prometheus formats response in exposition format
```

**Key Characteristics**:
- **Stateless**: No metric caching between scrapes
- **Synchronous**: Scrape duration includes PDU query time
- **Thread-safe**: Mutex prevents concurrent probes to same PDU

### 2.2 Metric Definitions

The collector exposes the following **gauge metrics** for power monitoring (all labeled with `pdu_id`). This list will grow as additional metrics are implemented:

| Metric Name | Type | Description | Unit | Labels |
|------------|------|-------------|------|--------|
| `pdu_power_watts` | Gauge | Real power consumption | Watts (W) | `pdu_id` |
| `pdu_apparent_va` | Gauge | Apparent power | Volt-Amperes (VA) | `pdu_id` |
| `pdu_reactive_var` | Gauge | Reactive power | Volt-Amperes Reactive (VAR) | `pdu_id` |
| `pdu_power_factor` | Gauge | Power factor (efficiency ratio) | Dimensionless (0-1) | `pdu_id` |

**Power Metrics Explained**:
- **Real Power (Watts)**: Actual power consumed and converted to work/heat
- **Apparent Power (VA)**: Total power flowing through the system (includes reactive component)
- **Reactive Power (VAR)**: Power oscillating between source and load (not performing useful work)
- **Power Factor**: Ratio of real power to apparent power (higher = more efficient, ideal = 1.0)

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
- `GET /metrics` - Prometheus exposition format (includes Go runtime metrics + PDU metrics)
- `GET /healthz` - Liveness probe (returns 200 OK)
- `GET /ready` - Readiness probe
- `GET /probe` - **Not yet implemented** - Future: Dynamic target probing

Additional endpoints will be added as new features are implemented.

## 4. Usage Example

### Collector Initialization

```go
import (
    "context"
    "net/url"
    "time"
    
    "github.com/prometheus/client_golang/prometheus"

    promcollector "github.com/SAP/rootstock/internal/collectors/prometheus"
)

// Parse PDU base URL
baseURL, _ := url.Parse("https://pdu01.example.com")

// Initialize Redfish client
redfishClient := redfish.NewClient(baseURL, logger, "admin", "password", 30*time.Second)
defer redfishClient.Close(context.Background())

// Login to Redfish
if err := redfishClient.Login(context.Background()); err != nil {
    log.Fatalf("Login failed: %v", err)
}

// Create and register collector
collector := promcollector.NewCollector(redfishClient, "1", logger)
prometheus.MustRegister(collector)
```

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'rootstock-pdu'
    scrape_interval: 5m
    scrape_timeout: 10s
    static_configs:
      - targets: ['localhost:8080']
        labels:
          environment: 'production'
          datacenter: 'dc1'
```

## 5. Future Enhancements

### Dynamic Target Probing

Implement `/probe?target=<pdu-id>`:

```yaml
scrape_configs:
  - job_name: 'pdu-probe'
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

- **Caching**: Short-lived cache (5-30s TTL) to reduce API load
- **Concurrent Collection**: Worker pool for multi-PDU deployments
- **Context Timeouts**: Proper deadline management
