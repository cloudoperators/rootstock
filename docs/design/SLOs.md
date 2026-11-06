# Service level objectives

To facilitate ease of use and ease of integration of rootstock into a larger
ecosystem of services and tools, this document proposes service level objectives
(SLOs) for the planned asynchronous, cache-backed collection architecture.
The performance tests, runtime SLO instrumentation, and cache-backed probe serving
described below are design goals, not currently implemented capabilities.

## Table of contents

- [Service level objectives](#service-level-objectives)
  - [Table of contents](#table-of-contents)
  - [Monitoring during development](#monitoring-during-development)
  - [Monitoring during runtime](#monitoring-during-runtime)
  - [Concrete SLOS](#concrete-slos)
    - [Metric Freshness](#metric-freshness)
    - [Collection Success Rate](#collection-success-rate)
    - [Device Coverage](#device-coverage)
    - [Probe Endpoint Availability](#probe-endpoint-availability)
    - [Probe Response Latency (p95)](#probe-response-latency-p95)

## Monitoring during development

To guard releases of rootstock against regression failures, we execute
standardised performance tests prior to every release. These tests play various
load and PDU response pattern scenarios to check the processing behaviour
(logic, and latency) of rootstock.

## Monitoring during runtime

During runtime, all below [SLOs](#concrete-slos) are actively monitored and
exposed as prometheus metrics at the dedicated rootstock `/metrics` endpoint.

## Concrete SLOS

We need to cover the logic of asynchronous metric-collection which cover

- scraping
- temporary storage
- serving

Ideally, SLO tests validate these aspects individually, to obtain cleaner
signals for debugging.

### Metric Freshness

`The percentage of configured devices whose cached metrics are no older than 2 × collection_interval.`

This SLO ensures that the collection loop is not silently breaking or falling
behind (which would indirectly break availability).

### Collection Success Rate

`The percentage of collection attempts (per device, per interval) that complete without error.`

This SLO tracks scraping reliability on a per-device basis. It allows to
identify problematic devices within the whole fleet.

### Device Coverage

`The percentage of configured devices that have had at least one successful collection in the last N × collection_interval.`

This SLO identifies devices that become permanently unavailable.

### Probe Endpoint Availability

`The percentage of /probe requests that return HTTP 200 with well-formed Prometheus text exposition.`

This SLO focusses on the serving side. Since responses are gathered from cache,
we are expecting a value close to 1 here.

### Probe Response Latency (p95)

`The p95 latency of /probe responses is below T ms.`

This SLO ensures acceptable latency on the serving side. Again, since responses
are gathered from cache, the latency values should be very low. Spikes could
point to serialization pressure, GC pauses, or lock contention on the in-memory
store — distinct from collection latency.
