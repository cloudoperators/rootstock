# Design

## 1. Overview

### 1.1 Background

Due to the EU legislation and increased efforts to implement sustainability
goals, this project aims to collect real-time energy consumption data for
datacenters and break it down to individual consumers.

### 1.2 Problem statement

Rootstock must collect sustainability-related metrics for available devices and
vendors so that sustainability targets for datacenters can be tracked.

Interesting data for collection are

- energy consumption [kWh]
- power [VA] [wiki](https://en.wikipedia.org/wiki/AC_power#Apparent_power)
  - apparent
  - reactive
  - active
  - powerfactor

### 1.3 Scope

The initial scope of the rootstock project is to collect energy consumption data
from power distribution units [PDU]s. This data will be made available in
Prometheus.

Future scope enhancements may be:

- energy consumption data for individual machines (either direct measurements or
  weighted distributions based on CPU utilisation)
- energy consumption data from the whole data-center to distribute loss data
  across all devices
- energy consumption calculation for VMs if possible
- energy consumption calculation for containers if possible
- all-in-one solution in 1 service from raw-data collection to the exposure of
  metrics via open-telemetry [OTEL] collectors
- additional OTEL collectors for metrics emmission

## 2. Requirements

To be generally useful, rootstock has to fulfil a set of business and technical
requirements. These are listed in this section.

### 2.1 Functional Requirements

Core functionality that must be implemented:

- **PDU Discovery and Configuration**: Support for static configuration of PDU
  targets. If possible, extend to dynamic PDU target discovery
- **Asynchronous scraping**: Periodically scrape current metrics from all
  available PDUs with
  [redfish protocol](https://redfish.dmtf.org/redfish/schema_index).
- **Ingestion endpoint**" Offer an ingestion endpoint for non-redfish PDU
  metrics.
- **Metric Processing**: Process incoming raw metrics to a standardised
  rootstock schema and enhance the output data where possible.
- **Metric Exposition**: Expose the result metrics via Prometheus and further
  OTEL collectors

### 2.2 Scalability

Since rootstock will be installed into every datacenter, it has to be able to
process all devices within 1 datacenter. Notice, that the longterm plan for
rootstock includes further types of devices like servers, routers, and other.
Hence, rootstock must be able to continuously scrape metrics for hundreds of
devices of various types.

We assume that

### 2.3 Security

### 2.4 Observability
