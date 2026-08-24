# OTel Collector Design

## Context

Rootstock's architecture includes three internal components: an HTTP Server, an OTel Collector, and a Redfish Client. This document covers the design decisions for the OTel Collector component — what it is, how it should be implemented, and the reasoning behind the chosen approach.

The OTel Collector sits between the HTTP Server and the Redfish Client:

```
Prometheus
    │
    │  GET /probe?target=<host>
    ▼
HTTP Server
    │
    ▼
OTel Collector  ←── this component
    │
    ▼
Redfish Client
    │
    │  HTTPS / Redfish API
    ▼
PDU Hardware
```

Its responsibilities:
- Receive raw telemetry from the Redfish Client
- Process and enrich the data (add labels, filter, normalize)
- Export metrics to one or more destinations

---

## The OTel Data Model: `pmetric.Metrics`

The OTel data model (`go.opentelemetry.io/collector/pdata/pmetric`) is used as the internal format for passing metrics between pipeline stages. It is a standardised in-memory container.

Using `pmetric.Metrics` as the internal format means:
- Any OTel-compatible exporter can consume it without conversion. (needs to be checked)
- Switching or adding export destinations only requires a new exporter — the collection logic does not change

---

## Two Approaches Considered

### Approach A — OTel Collector builder SDK

Use `go.opentelemetry.io/collector/component` and `consumer.Metrics` to implement the pipeline stages as formal OTel Collector components. `main.go` would use `otelcol.NewCommand()` or `service.New()` to start a pipeline engine that manages component lifecycle (Start/Shutdown) and wires stages together.

The pipeline engine drives the collection: it calls `Start()` on the receiver, which launches a background goroutine that collects continuously and pushes data via `consumer.Metrics.ConsumeMetrics()` to the next stage.

```
engine.Start()
  → receiver.Start() → goroutine runs forever, ticks every 30s
      → fetches from PDU
      → r.next.ConsumeMetrics(ctx, metrics)  → processor → exporter
```

**Advantages:**
- Built for continuous push to multiple backends (OTLP, OpenSearch, etc.)
- Engine manages graceful shutdown, error recovery, back-pressure
- Standard OTel Collector architecture — well-documented pattern

**Disadvantages for v1:**
- Engine is designed for push-based, continuously-running receivers
- Conflicts with Prometheus multi-target pull pattern (per-request, on-demand)
- Requires a cache layer between the pipeline and the HTTP handler
- PDU targets must be pre-configured at startup — dynamic targets from Prometheus are not supported without additional complexity
- Session renewal required in the background loop (Redfish sessions expire)
- Significant additional code before anything works end-to-end

### Approach B — Per-request pipeline (chosen for v1)

Use `pmetric.Metrics` as the internal format but implement the pipeline stages as plain Go functions. The HTTP handler drives the pipeline per scrape request — no engine, no background goroutines.

```
GET /probe?target=pdu01
  → receiver.Receive(ctx)  →  pmetric.Metrics
  → processor.Process(ctx, metrics)
  → exporter.Export(ctx, metrics)
  → HTTP response
```

**Advantages:**
- Fits the Prometheus multi-target pull pattern naturally
- Any target Prometheus sends is handled dynamically — no pre-configuration
- Always-live data — fetched at the moment of the scrape
- No cache layer, no thread safety issues, no session renewal in background
- Simpler to implement and reason about
- `receiver.Receive()` is reusable — same function can be called from a background goroutine in v2

**Disadvantages:**
- Does not support continuous push to other backends on its own
- Background collection for non-Prometheus backends requires additional work in v2

---

## Design Decision


## Pipeline Stages

### Receiver

Fetches raw data from a single PDU via the Redfish API and converts it to `pmetric.Metrics`.

- Opens a Redfish session (Login)
- Fetches from `/redfish/v1/PowerEquipment/RackPDUs` and related endpoints
- Converts JSON response to `pmetric.Metrics` data points
- Closes the session (Close) — always, via `defer`
- Stateless: safe to call once per request, no shared state

### Processor

Enriches `pmetric.Metrics` with attributes that apply to all metrics from this rootstock instance.

- Adds `service.name` (standard OTel semantic convention)
- Can add cluster name, environment, or other infrastructure labels
- Does not interact with external systems

### Exporter

Reads `pmetric.Metrics` and writes to a destination.

Planned implementations:


---

## Open Questions

