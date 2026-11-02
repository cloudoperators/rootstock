#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# Run the in-code Go performance benchmarks (processing pipeline + result cache)
# and print a short, protocol-agnostic guide for interpreting the results.
#
# All configuration lives here as ROOTSTOCK_BENCH_* environment variables so the
# Makefile entrypoint stays thin. Override them on the command line, e.g.:
#   make test-performance ROOTSTOCK_BENCH_COUNT=6 ROOTSTOCK_BENCH_TIME=5s
#   ROOTSTOCK_BENCH_COUNT=6 hack/performance-test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Benchmark configuration (overridable via the environment).
ROOTSTOCK_BENCH_TIME="${ROOTSTOCK_BENCH_TIME:-2s}"        # wall-time budget per benchmark
ROOTSTOCK_BENCH_COUNT="${ROOTSTOCK_BENCH_COUNT:-1}"       # repeats; raise (e.g. 6) when feeding benchstat
ROOTSTOCK_BENCH_CPU="${ROOTSTOCK_BENCH_CPU:-1}"           # pin GOMAXPROCS for reproducibility
ROOTSTOCK_BENCH_TIMEOUT="${ROOTSTOCK_BENCH_TIMEOUT:-10m}" # hard ceiling so the run can never hang
ROOTSTOCK_BENCH_OUT="${ROOTSTOCK_BENCH_OUT:-bench.txt}"

# Packages to benchmark. Defaults to the whole internal tree so any package that
# gains a Benchmark* function is picked up automatically (auto-discovery). The
# trade-off vs a curated allowlist: auto-discovery can't go stale, but it also
# compiles every internal package and will silently include any future
# benchmark. Override ROOTSTOCK_BENCH_PKGS to pin an explicit allowlist instead.
ROOTSTOCK_BENCH_PKGS="${ROOTSTOCK_BENCH_PKGS:-./internal/...}"

cd "${REPO_ROOT}"

# Run benchmarks only (-run=^$ excludes unit tests); tee keeps bench.txt clean
# for benchstat (the interpretation guide below is printed to stdout only).
go test \
	-run='^$' \
	-bench=. \
	-benchmem \
	-benchtime="${ROOTSTOCK_BENCH_TIME}" \
	-count="${ROOTSTOCK_BENCH_COUNT}" \
	-cpu="${ROOTSTOCK_BENCH_CPU}" \
	-timeout="${ROOTSTOCK_BENCH_TIMEOUT}" \
	"${ROOTSTOCK_BENCH_PKGS}" | tee "${ROOTSTOCK_BENCH_OUT}"

cat <<'EOF'
How to read the results above:
  ns/op      average time per operation   (lower is faster)
  B/op       heap memory per operation    (lower is leaner)
  allocs/op  heap allocations per op      (watch this first for regressions)
Absolute values are machine-specific (see goos/goarch/cpu); compare runs on identical hardware. Scaled benchmarks should grow ~linearly with the item count.
EOF
