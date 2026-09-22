# Design Document: Rootstock

## Table of Contents

- [Design Document: Rootstock](#design-document-rootstock)
  - [Table of Contents](#table-of-contents)
  - [1. Overview](#1-overview)
    - [1.1 Problem statement](#11-problem-statement)
    - [1.2 Scope](#12-scope)
    - [1.3 Background](#13-background)
  - [2. Goals and Non-Goals](#2-goals-and-non-goals)
    - [2.1 Goals](#21-goals)
      - [2.1.0 Comprehensive Energy Metrics Monitoring](#210-comprehensive-energy-metrics-monitoring)
      - [2.1.1 Scalability](#211-scalability)
      - [2.1.2 Multi-Vendor Support](#212-multi-vendor-support)
      - [2.1.3 Security and Production Readiness](#213-security-and-production-readiness)
        - [SSRF Protection (C-1 Control):](#ssrf-protection-c-1-control)
        - [Credential Security:](#credential-security)
      - [2.1.4 Performance Optimization](#214-performance-optimization)
      - [2.1.5 Ready-to-Go Operation](#215-ready-to-go-operation)
      - [2.1.6 Observability](#216-observability)
    - [2.2 Non-Goals](#22-non-goals)
  - [3. Requirements](#3-requirements)
    - [3.1 Functional Requirements](#31-functional-requirements)
    - [3.2 Non-Functional Requirements](#32-non-functional-requirements)
      - [Scalability Requirements](#scalability-requirements)
      - [Security Requirements](#security-requirements)
      - [Maintainability Requirements](#maintainability-requirements)
  - [4. System Architecture](#4-system-architecture)
    - [4.1 High-Level Architecture](#41-high-level-architecture)
    - [4.2 Component Design](#42-component-design)
      - [4.2.1. HTTP Server](#421-http-server)
      - [4.2.2. OTEL Collector](#422-otel-collector)
      - [4.2.3. Prometheus Collector](#423-prometheus-collector)
      - [4.2.4. Redfish Client](#424-redfish-client)
      - [4.2.5. PDU Discovery and Configuration](#425-pdu-discovery-and-configuration)
    - [4.3 Data Flow](#43-data-flow)
      - [4.3.1 Prometheus Probing Flow](#431-prometheus-probing-flow)
      - [4.3.2 OTLP Compatible Vendor Flow](#432-otlp-compatible-vendor-flow)
  - [5. API Design](#5-api-design)
    - [5.1 HTTP Server Endpoints (Rootstock)](#51-http-server-endpoints-rootstock)
      - [5.1.1 Metrics Endpoint](#511-metrics-endpoint)
      - [5.1.2 Health and Readiness Endpoints](#512-health-and-readiness-endpoints)
    - [5.2 Redfish Client API (PDU Side)](#52-redfish-client-api-pdu-side)
      - [5.2.1 Redfish Endpoints Used](#521-redfish-endpoints-used)
      - [5.2.2 Authentication](#522-authentication)
  - [6. Security Considerations](#6-security-considerations)
  - [7. Performance and Scalability](#7-performance-and-scalability)
    - [7.1 Performance Targets](#71-performance-targets)

---

## 1. Overview

### 1.1 Problem statement

Rootstock should solve the issue of collecting sustainability-related metrics
for available devices (starting with power distribution units [PDUs]). It is
supposed to collect relevant metrics for multiple vendors and expose them for
Prometheus and other (open-telemetry) targets.

### 1.2 Scope

The scope might eventually change. But for now we only want to collect basic
energy metrics for PDUs. We will start with Panduit and eventually add more
vendors. We will use Redfish protocol to get those and store them in Prometheus
and eventually visualize in Perses. We want to convert data to Prometheus and
let Prometheus scrape the data.

### 1.3 Background

Due to the EU legislation we want to build a project that allows others to
collect sustainability metrics for PDUs and it allows others to have a better
overview of their energy metrics for their servers.

## 2. Goals and Non-Goals

### 2.1 Goals

We want to monitor hundreds of racks at scale. Provide comprehensive real-time
monitoring of PDUs for variety of vendors starting with Panduit.

#### 2.1.0 Comprehensive Energy Metrics Monitoring

Scrape meaningful energy and sustainability metrics including:

- Power consumption (Watts)
- Energy usage (kWh)
- Power factor

#### 2.1.1 Scalability

- Monitor 100+ racks
- This exporter service will handle hundreds of racks
- Multi target pattern, one pod many targets

#### 2.1.2 Multi-Vendor Support

Eventually we want to support various vendors starting with Panduit. Handling
vendor-specific Redfish requirements.

#### 2.1.3 Security and Production Readiness

##### SSRF Protection (C-1 Control):

- Prevent cloud metadata attacks (169.254.169.254)
- Hostname whitelist enforcement
- Reject IP addresses, URLs, ports
- Domain suffix validation

##### Credential Security:

- Kubernetes secrets integration
- Vault support for credential injection
- Never log credentials
- No credential exposure in metrics

#### 2.1.4 Performance Optimization

Efficient monitoring without overload. Not querying too often, reducing load,
faster scrapes.

#### 2.1.5 Ready-to-Go Operation

Making it easy to run. Helm chart for easy deployment, ServiceMonitor for
Prometheus Operator. RBAC and resource limits and health checks.

#### 2.1.6 Observability

Self monitoring metrics, scrape duration. Alerting for critical scenarios if
servers are down.

### 2.2 Non-Goals

## 3. Requirements

### 3.1 Functional Requirements

Core functionality that must be implemented:

- **PDU Discovery and Configuration**: Support for static configuration of PDU
  targets with validation and whitelisting
- **Scrape Request Handling**: Receiving scrape requests from Prometheus with
  multi-target pattern support
- **Redfish Communication**: Exporting PDU energy metrics via Redfish protocol
- **Metric Conversion**: Converting Redfish responses to Prometheus format
- **Metric Exposition**: Returning formatted metrics to Prometheus

### 3.2 Non-Functional Requirements

#### Scalability Requirements

- Support 100+ PDU targets from a single exporter instance

#### Security Requirements

- TLS/HTTPS for all Redfish communications
- Secure credential storage (Kubernetes secrets, Vault) TBD

#### Maintainability Requirements

- Clear separation of concerns (HTTP server, Redfish client, metrics processing)
- Vendor-agnostic Redfish client design
- Comprehensive logging and observability
- Self-monitoring metrics (scrape duration, error rates)

## 4. System Architecture

### 4.1 High-Level Architecture

**System overview diagram:**

```
┌────────────────────┐         ┌──────────────────┐         ┌──────────────────┐
│                    │         │                  │         │                  │
│   PROMETHEUS       │ ──────► │  ROOTSTOCK       │ ──────► │   PDU/HARDWARE   │
│   (Observer)       │  Pull   │                  │  Query  │   (Data Source)  │
│                    │         │  (Translator)    │         │                  │
└────────────────────┘         └──────────────────┘         └──────────────────┘
  "I want metrics"              "I'll get them"             "Here's my status"

  Stores time-series            Converts formats            Physical servers
                                Routes to targets           Panduit (eventually other vendors)
                                Handles security            Redfish API
```

**Major components and their relationships:**

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          Monitoring Network                              │
│                                                                          │
│  ┌───────────────────────┐         ┌──────────────────────────┐          │
│  │    Prometheus         │         │   OTLP Compatible        │          │
│  │    (or other vendors) │         │   Vendor (Dynatrace,     │          │
│  │  Scrape /metrics      │         │   Datadog, New Relic)    │          │
│  └────────┬──────────────┘         └──────────────────────────┘          │
│           │ HTTP GET /metrics                 ▲                          │
│           │                                   │ OTLP Push                │
│           ▼                                   │                          │
│  ┌────────────────────────────────────────────┬────────────────────┐     │
│  │                        Rootstock           │                    │     │
│  │                                            │                    │     │
│  │  ┌────────────────┐     ┌──────────────┐   │ ┌──────────────┐   │     │
│  │  │  HTTP Server   │────▶│  Prometheus  │   │ │     OTEL     │   │     │
│  │  │                │     │  Collector   │   └─┤  Collector   │   │     │
│  │  │  /metrics      │     │              │     │              │   │     │
│  │  │  /power        │     └──────┬───────┘     └──────┴───────┘   │     │
│  │  │                │            │                    │           │     │
│  │  └────────────────┘            │                    │           │     │
│  │                                │                    │           │     │
│  │                                │                    │           │     │
│  │                                │                    │           │     │
│  │                                ▼                    ▼           │     │
│  │                         ┌──────────────────────────────┐        │     │
│  │                         │       Redfish Client         │        │     │
│  │                         │                              │        │     │
│  │                         │ - HTTP/HTTPS                 │        │     │
│  │                         │ - Auth                       │        │     │
│  │                         │ - TLS                        │        │     │
│  │                         │ - Timeouts                   │        │     │
│  │                         │ - JSON decode                │        │     │
│  │                         └──────────────┬───────────────┘        │     │
│  │                                        │                        │     │
│  └────────────────────────────────────────┼────────────────────────┘     │
│                                            HTTPS / Redfish               │
└────────────────────────────────────────────────────────┼─────────────────┘
                                                         │
                            ┌────────────────────────────┼────────────────┐
                            │                            │                │
                            ▼                            ▼                ▼
                     ┌─────────────┐            ┌─────────────┐   ┌─────────────┐
                     │    PDU 1    │            │    PDU 2    │   │    PDU N    │
                     │             │            │             │   │             │
                     │  Redfish    │            │  Redfish    │   │  Redfish    │
                     │     API     │            │     API     │   │     API     │
                     └─────────────┘            └─────────────┘   └─────────────┘
```

### 4.2 Component Design

#### 4.2.1. HTTP Server

**Responsibilities:**

- Accept incoming scrape requests from Prometheus
- Route requests to appropriate collector endpoint
- Validate and sanitize input parameters
- Handle graceful shutdown

#### 4.2.2. OTEL Collector

- transport, process, enrich, filter, batch, route, export

#### 4.2.3. Prometheus Collector

For the Prometheus probing route, we implement a Prometheus collector that is
compatible with Prometheus's pull-based scraping mechanism. More detailed design
in [PROMETHEUS.md](PROMETHEUS.md).

#### 4.2.4. Redfish Client

The Redfish client is the core HTTP communication layer for interacting with PDU
devices using the Redfish API.

**Responsibilities:**

- Establish HTTPS connections to PDU endpoints
- Handle authentication (Basic Auth, Session-based)
- Execute Redfish API calls to retrieve power metrics
- Parse JSON responses from Redfish API
- Handle timeouts and retries

**Supported Vendors (Initial):**

- Panduit (primary target)
- Extensible for other Redfish-compliant PDUs

#### 4.2.5. PDU Discovery and Configuration

**Overview:** For the initial implementation, we use static configuration with
validation.

**Configuration Approach:**

- **Static Configuration**: PDU targets defined in YAML configuration file
- **Multi-Target Pattern**: Prometheus passes target parameter in scrape request

### 4.3 Data Flow

#### 4.3.1 Prometheus Probing Flow

**Complete Flow:**

1. **Prometheus → Rootstock**: Prometheus scrapes `/metrics?target=<pdu-host>`
2. **HTTP Server**: Receives request and validates target parameter
3. **Prometheus Collector**: Checks cache for `<pdu-host>` metrics
   - **Cache Hit**:
     - Returns cached metrics immediately
     - Skips to step 8
   - **Cache Miss** (no cached data or expired):
     - Proceeds to step 4
4. **Redfish Client**: Establishes HTTPS connection to PDU and handles
   authentication
5. **Redfish API Call**: The Redfish client queries the appropriate Redfish
   endpoints to retrieve PDU metrics
6. **Response Processing**: The Redfish client parses the JSON response and
   returns the relevant data to the collector
7. **Cache Update**: Stores fetched metrics in cache with timestamp for future
   requests
8. **Metric Conversion**: Converts Redfish data to Prometheus format in
   Prometheus Collector
9. **HTTP Response**: Returns Prometheus metrics to Prometheus server

For the POC we would leave out the Cache implementation

#### 4.3.2 OTLP Compatible Vendor Flow

**Complete Flow:**

1. **OTLP Collector**: Initiates collection of power and energy metrics
2. **Redfish Client**: Establishes HTTPS connection to PDU and handles
   authentication
3. **Redfish API Call**: The Redfish client queries the appropriate Redfish
   endpoints to retrieve PDU metrics
4. **Response Processing**: The Redfish client parses the JSON response and
   returns the relevant data to the collector
5. **Metric Conversion**: The OTLP Collector converts the Redfish data into
   OpenTelemetry pmetric format
6. **OTLP Push**: The OTEL Collector exports the metrics using OTLP to the
   compatible telemetry vendor

## 5. API Design

### 5.1 HTTP Server Endpoints (Rootstock)

These endpoints are exposed by Rootstock for Prometheus and other monitoring
tools to scrape.

#### 5.1.1 Metrics Endpoint

```
GET /metrics?target=<host>
```

**Description:** Main endpoint for Prometheus scraping using multi-target
pattern.

**Response:** Prometheus exposition format with PDU metrics

#### 5.1.2 Health and Readiness Endpoints

```
GET /health   # Liveness probe
GET /ready    # Readiness probe
```

### 5.2 Redfish Client API (PDU Side)

We want the Redfish client to be device agnostic, but we will start implementing
for Panduit PDUs.

#### 5.2.1 Redfish Endpoints Used

The following Redfish API endpoints will be queried on PDU devices:

**Service Root:**

```
GET /redfish/v1/
```

**Power Equipment Collection:**

```
GET /redfish/v1/PowerEquipment
```

**Rack PDUs Collection:**

```
GET /redfish/v1/PowerEquipment/RackPDUs
```

For more information about redfish client, check out docs/REDFISH.md

#### 5.2.2 Authentication

**Supported Authentication Methods:** TBD

## 6. Security Considerations

- Authentication and authorization mechanisms
- Data encryption
- Security vulnerabilities and mitigations
- Compliance requirements TBD

## 7. Performance and Scalability

### 7.1 Performance Targets

- Response time requirements
- Throughput requirements TBD
