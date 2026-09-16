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