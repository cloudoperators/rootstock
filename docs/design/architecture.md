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

## 2. Technical Requirements

To be generally useful, rootstock has to fulfil a set of technical requirements
on top of the business logic requirements.

### 2.1 Scalability

### 2.2 Security

### 2.3 Observability
