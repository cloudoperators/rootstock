# Design Document: Roostock

## 1. Overview
### 1.1 Problem statement
Roostock should solve the issue of collecting sustainibility metrics for PDUs. This service is suppose to collect these metrics for multiple vendors and store them in Prometheus. 

### 1.2 Scope
The scope might eventually change. But for now we only want to collect basic energy metrics for PDUs. We wills start with Panduit and eventually add more vendors. We will use redfish protocol to get those and store them in Prometheus and eventually visualize in Perses. We want to convert data to Prometheus and let Prometheus scrape the data.

### 1.3 Background
Due to the EU legislation we want to build a project that allows others to collect sustainability metrics for PDUs and it allows others to have a better overview of their energy metrics for their servers. 

## 2. Goals and Non-Goals
### 2.1 Goals
We want to monitor hundreds of racks at scale. Provide comprehensive real-time monitoring of PDUs for variety of vendors starting with Panduit. 

#### 2.1.0 Comprehensive Energy metrics Monitoring
Scrape meaningful energy and sustainibility metrics
Here list some ....
- Power

#### 2.1.1 Scalability
- Monitor 100+ racks
- This exporter service will handle hundreds of racks
- Multi target pattern, one pod many targets
- Efficient resource usage

#### 2.1.2 Multi vendor support
Eventually we want to support various vendors starting with Panduit. Handlign vendor specific redfish requirements.

#### 2.1.3 Security and production Readiness
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

#### 2.1.5 Ready to go operation 
Making it easy to run. Helm chart for easy deployment, ServiceMonitor for Promethesu Operator. RBAC? and resourc elimits and helth checks.

#### 2.1.6 Observability
Self monitoring metrics, scrape duration. Alerting for critical scenarios if servers are down. 

### 2.2 Non-Goals

## 3. Requirements
### 3.1 Functional Requirements
- Core functionality that must be implemented

- Receiving scrape requests from Prometheus
- Exporting PDU energy metrics via redfish protocol
- Metric conversion to Prometheus format
- Return them to Prometheus

- User stories or use cases

### 3.2 Non-Functional Requirements
- Performance requirements
- Scalability requirements
- Security requirements
- Reliability/availability requirements
- Maintainability requirements

## 4. System Architecture
### 4.1 High-Level Architecture
- System overview diagram
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

- Major components and their relationships

                         Monitoring Network
┌──────────────────────────────────────────────────────────────────────┐
│                                                                      │
│   ┌─────────────────────┐                                            │
│   │    Prometheus       │                                            │
│   │    (or other vedors)│                                            │
│   │  Scrape /metrics    │                                            │
│   └────────┬────────────┘                                            │
│            │ HTTP                                                    │
│            │ GET /metrics                                            │
│            ▼                                                         │
│   ┌──────────────────────────────────────────────────────────────┐   │
│   │                  Rootstock                                   │   │
│   │                                                              │   │
│   │  ┌────────────────┐      ┌──────────────────────────────┐    │   │
│   │  │ HTTP Server    │      │ OTEL Collector               │    │   │
│   │  │                │─────▶│                              │    │   │
│   │  │ /metrics       │      │ - Receiver -> Processor      │    │   │
│   │  │ /power         │      │  ->  Exporter                |    |   |
|   |  |                |      | - Metric collection          │    │   │
│   │  │                │      │ - Metric normalization       │    │   │
│   │  └────────────────┘      └──────────────┬───────────────┘    │   │
│   │                                         │                    │   │
│   │                                         ▼                    │   │
│   │                         ┌──────────────────────────────┐     │   │
│   │                         │       Redfish Client         │     │   │
│   │                         │                              │     │   │
│   │                         │ - HTTP/HTTPS                 │     │   │
│   │                         │ - Authentication             │     │   │
│   │                         │ - TLS                        │     │   │
│   │                         │ - Timeouts                   │     │   │
│   │                         │ - JSON decoding              │     │   │
│   │                         └──────────────┬───────────────┘     │   │
│   │                                        │                     │   │
│   └────────────────────────────────────────┼───────────────────-─┘   │
│                                            │ HTTPS / Redfish         │
└────────────────────────────────────────────┼─────────────────────-───┘
                                             │
                       ┌─────────────────────┼─────────────────────┐
                       │                     │                     │
                       ▼                     ▼                     ▼
                ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
                │    PDU 1    │       │    PDU 2    │       │    PDU N    │
                │             │       │             │       │             │
                │  Redfish    │       │  Redfish    │       │  Redfish    │
                │     API     │       │     API     │       │     API     │
                └─────────────┘       └─────────────┘       └─────────────┘

### 4.2 Component Design
#### 4.2.1. HTTP Server 

**Responsibilities:**
- Accept incoming scrape requests from Prometheus
- Route requests to appropriate collector endpoint
- Validate and sanitize input parameters
- Handle graceful shutdown

#### 4.2.2. OTEL Collector
- transport, process, enrich, filter, batch, route, export

#### 4.2.3. Redfish client
The redfish client is the core HTTP communication layer for interacting with PDU devices using the Redfish API.
Mention device vendors.

### 4.3 Data Flow
- Prometheus scrape -> HTTP Client -> Redfish client -> Metrics Processing -> HTTP response (Prometheus server or other veondors) 


## 5. API Design

### 5.1 HTTP Server for Prometheus
There will be endpoints exposed fo prometheus (or other vendor to scrape)


### 5.2 Redfish Client
We want the Redfish client to be device agnostic, but we will start implementing for Panduit PDUs.

We will start with the following endpoints:
- /redfish/v1/PowerEquipment
- /redfish/v1/PowerEquipment/RackPDUs


- Authentication/authorization ?



## 6. Security Considerations
- Authentication and authorization mechanisms
- Data encryption
- Security vulnerabilities and mitigations
- Compliance requirements

## 7. Performance and Scalability
### 7.1 Performance Targets
- Response time requirements
- Throughput requirements



