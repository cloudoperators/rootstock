# Performance testing

This document describes the performance-testing approach for rootstock and the
metrics we intend to measure.

## Open questions

- if we need to adhere to product standards it would make sense to either mirror
  the repository or somehow direct the internal tools at the public repository

- do we want to performance test the prometheus collector (and set the result
  cache (buffer) static)?
  - measures peak request per second and response time
  - requires feature flag to set the cache static
  - security risk if you enable the flag in production -> risk / reward
    potentially low

## Notes

- do we need to adhere to SAPs product standards
  - We don't need to adhere to SLC (pipeline) standards. SCI is exempt. Also,
    this project is not supposed to be owned by SAP in the future. We want
    transparent pipeline runs and builds to open registries
- should we run the tests on a public github runner?
  - yes, for now. running tests on a public virtualized environment will
    probably cause erratic absolute response measurements. depending on how
    erratic it is, we can consider self-hosting our own runner later on.

test frequency -> fail early. which stage the tests run depends on how expensive
(runtime) they are if cheaper, higher up

- on-commit
- on-automerge
- on-release

we will test two initial cases for rootstock

we run two test cases directly at the code level. At the result cache, we verify
the performance of the processing pipeline using a static set of mocked

1. SNMP PDUs
2. redfish PDUs

later, we can test at the prometheus collector using a statically filled result
cache

testing against the deployed container in a kind cluster is currently off the
table as we deem it not necessary to benchmark test relevant components and
measuring relative deviations.

## In-code benchmark harness

The native Go benchmarks live beside the code they measure and are driven by a
reusable harness under `internal/perf`:

- `internal/perf/mockpdu` — a fixture-backed implementation of `redfish.Client`
  (the "mock PDU" edge). Fixtures are a copy of the DMTF Redfish PDU mockup
  (`testdata/pdu-mockup`) embedded via `go:embed`, decoded once at construction
  so per-iteration cost reflects processing, not JSON parsing or network I/O.
- `internal/perf/harness.go` — the `Pipeline` that wires the three measured
  stages together: mock PDU source → `Processor` → result cache. The benchmark
  observes results landing in the abstract result cache, which is the
  measurement point described above.

### Pending real components

The processing-layer stages in
[docs/design/architecture.md](../design/architecture.md) (ingestion queue,
processor, result queue/cache wiring) and the SNMP scraper do **not** exist yet
and are intentionally **not** implemented here. The harness instead exposes a
single extension seam — the `perf.Processor` interface — so that once a real
processor is built it can be dropped into the existing benchmarks without
rewriting the harness. Until then, `perf.DefaultProcessor` provides a
representative field-mapping transform so the tooling is exercisable end-to-end.

SNMP benchmarks are pending the SNMP package.

### What is benchmarked today

- `BenchmarkPipeline` / `BenchmarkPipelineScale` / `BenchmarkPipelineParallel`
  (`internal/perf`): mock PDU → process → result cache, including mass-PDU
  scaling (`pdus=1/10/100/1000`) and concurrent processing.
- `BenchmarkCacheSet` / `BenchmarkCacheGet` and their `Parallel` variants
  (`internal/collectors/cache`): the result cache under sequential and
  concurrent load.

### Running the benchmarks

```sh
make test-performance
```

This runs benchmarks only (unit tests are excluded via `-run=^$`), writes the
benchstat-compatible output to `bench.txt`, and never touches the network or a
mockup container. Tunables (with defaults) can be overridden on the command
line, e.g.:

```sh
make test-performance BENCHTIME=5s BENCHCOUNT=6 BENCHCPU=1
```

`BENCHCPU=1` pins `GOMAXPROCS` for reproducibility; `BENCHTIMEOUT` is a hard
ceiling so a run can never hang the pipeline. A CI quality gate and a stored
baseline for regression comparison are deliberately deferred until the benchmark
runtimes are known.
