# Design Document: Rootstock

## 1. Overview
### 1.1 Problem statement
Rootstock should solve the issue of collecting sustainability metrics for PDUs. This service is supposed to collect these metrics for multiple vendors and store them in Prometheus. 

### 1.2 Scope
The scope might eventually change. But for now we only want to collect basic energy metrics for PDUs. We will start with Panduit and eventually add more vendors. We will use Redfish protocol to get those and store them in Prometheus and eventually visualize in Perses. We want to convert data to Prometheus and let Prometheus scrape the data.

### 1.3 Background
Due to the EU legislation we want to build a project that allows others to collect sustainability metrics for PDUs and it allows others to have a better overview of their energy metrics for their servers. 

## 2. Goals and Non-Goals
### 2.1 Goals
We want to monitor hundreds of racks at scale. Provide comprehensive real-time monitoring of PDUs for variety of vendors starting with Panduit. 

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
Eventually we want to support various vendors starting with Panduit. Handling vendor-specific Redfish requirements.

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
Efficient monitoring without overload. Not querying too often, reducing load, faster scrapes.

#### 2.1.5 Ready-to-Go Operation 
Making it easy to run. Helm chart for easy deployment, ServiceMonitor for Prometheus Operator. RBAC and resource limits and health checks.

#### 2.1.6 Observability
Self monitoring metrics, scrape duration. Alerting for critical scenarios if servers are down. 

### 2.2 Non-Goals

## 3. Requirements
### 3.1 Functional Requirements
Core functionality that must be implemented:

- **PDU Discovery and Configuration**: Support for static configuration of PDU targets with validation and whitelisting
- **Scrape Request Handling**: Receiving scrape requests from Prometheus with multi-target pattern support
- **Redfish Communication**: Exporting PDU energy metrics via Redfish protocol
- **Metric Conversion**: Converting Redfish responses to Prometheus format
- **Metric Exposition**: Returning formatted metrics to Prometheus

### 3.2 Non-Functional Requirements

#### Scalability Requirements
- Support 100+ PDU targets from a single exporter instance

#### Security Requirements
- TLS/HTTPS for all Redfish communications
- Secure credential storage (Kubernetes secrets, Vault)
TBD

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

#### 4.2.3. Redfish Client
The Redfish client is the core HTTP communication layer for interacting with PDU devices using the Redfish API.

**Responsibilities:**
- Establish HTTPS connections to PDU endpoints
- Handle authentication (Basic Auth, Session-based)
- Execute Redfish API calls to retrieve power metrics
- Parse JSON responses from Redfish API
- Handle timeouts and retries

**Supported Vendors (Initial):**
- Panduit (primary target)
- Extensible for other Redfish-compliant PDUs

#### 4.2.4. PDU Discovery and Configuration

**Overview:**
For the initial implementation, we use static configuration with validation.

**Configuration Approach:**
- **Static Configuration**: PDU targets defined in YAML configuration file
- **Multi-Target Pattern**: Prometheus passes target parameter in scrape request

### 4.3 Data Flow

**Complete Flow:**
1. **Prometheus → Rootstock**: Prometheus scrapes `/metrics?target=xyz
2. **HTTP Server**: Receives request, validates target parameter
3. **Redfish Client**: Establishes HTTPS connection to PDU, authenticates
4. **Redfish API Call**: Queries
5. **Response Processing**: Parse JSON response
6. **OTEL Collector** (optional): Process, enrich, transform metrics
7. **Metric Conversion**: Convert Redfish data to Prometheus format
8. **HTTP Response**: Return Prometheus metrics to Prometheus server

## 5. API Design

### 5.1 HTTP Server Endpoints (Rootstock)

These endpoints are exposed by Rootstock for Prometheus and other monitoring tools to scrape.

#### 5.1.1 Metrics Endpoint
```
GET /metrics?target=<host>
```

**Description:** Main endpoint for Prometheus scraping using multi-target pattern.

**Response:** Prometheus exposition format with PDU metrics

#### 5.1.2 Health and Readiness Endpoints
```
GET /health   # Liveness probe
GET /ready    # Readiness probe
```


### 5.2 Redfish Client API (PDU Side)

We want the Redfish client to be device agnostic, but we will start implementing for Panduit PDUs.

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
**Supported Authentication Methods:**
TBD

## 6. Security Considerations
- Authentication and authorization mechanisms
- Data encryption
- Security vulnerabilities and mitigations
- Compliance requirements
TBD

## 7. Performance and Scalability
### 7.1 Performance Targets
- Response time requirements
- Throughput requirements
TBD



