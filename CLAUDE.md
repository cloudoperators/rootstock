# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What this project is

**rootstock** is a Go-based Prometheus exporter that collects energy and power
metrics from datacenter PDUs (Power Distribution Units) via the
[Redfish](https://redfish.dmtf.org/) protocol. It exposes a multi-target probe
HTTP endpoint so Prometheus can scrape individual PDUs on demand. It deploys as
a Kubernetes workload and integrates with the
[Greenhouse](https://github.com/cloudoperators/greenhouse) plugin ecosystem.

## Commands

```bash
# Build
make build              # compile static binary to bin/rootstock
make docker-build       # build container image (auto-detects arch)

# Test
make test               # alias for test-unit (unit tests with race detection and coverage)
make test-integration   # starts Redfish mockup container, runs integration tests, stops container
make test-changelog     # run changelog generation test suite

# Run a single integration test (requires the Redfish mockup server)
go test -tags=integration ./internal/redfish/... -run '^TestLogin$' -v

# Lint (use the golangci-lint version selected in .github/workflows/ci.yaml)
make lint               # lint-go + lint-md + lint-shell + lint-helm, then go mod tidy
make lint-go            # golangci-lint run --fix
make lint-md            # prettier on markdown
make lint-shell         # shellcheck on shell scripts
make lint-helm          # helm lint on the chart

# Run locally (after 1-time setup in docs/local_development.md)
cat > ./tmp/redfish_pdus.yaml <<'EOF'
pdu1:
  username: redfish
  password: secret
EOF
go run main.go run --config ./tmp/.rootstock.yaml --config-folder ./tmp

# Local Kubernetes via kind (requires docker, kind, kubectl, helm)
make kind-up            # create cluster, load image, deploy mockup + Helm chart
make kind-validate      # probe the running pod
make kind-delete        # tear down the cluster

# Prometheus local dev
make prometheus-local-run   # build + start mockup + start Prometheus + run binary
make prometheus-local-cleanup

# Release tooling
make generate-changelog     # regenerates CHANGELOG.md from git history via git-cliff
make generate-version       # bumps VERSION.yaml to match
```

## Architecture

### Request flow

Rootstock exposes a **multi-target probe pattern** — Prometheus scrapes
`/probe?target=<pdu-url>&labels=<pdu-id>` rather than `/metrics` directly. For
each probe request, the server:

1. Looks up PDU credentials from the in-memory credentials map
2. Creates a per-request `redfish.Client` pointing at the target URL
3. Creates a per-request `prometheus.Collector` backed by that client
4. Registers the collector in a temporary `prometheus.Registry`
5. Serves the Prometheus text format back to the caller

Each scrape creates a fresh Redfish client and collector. The collector calls
`GetPERackPDUMetrics` and `GetPERackPDU` directly, without calling `Login` or
`Close`.

### Package layout

```
main.go                              entrypoint; sets build-time version vars
cmd/                                 Cobra CLI wiring
  new.go                             root command, viper config, logger setup
  run.go                             `run` subcommand; watches redfish_pdus.yaml; owns server lifecycle
  mountfile.go                       live-reloading file watcher (fsnotify) used for credentials
  utils.go                           CLI parameter helper (flag/env/config binding via viper)
internal/
  communication/rest/server.go       chi HTTP server; /probe, /metrics, /healthz, /ready
  collectors/prometheus/collector.go prometheus.Collector implementation; calls redfish.Client
  collectors/cache/                  generic thread-safe in-memory cache (used for future caching)
  redfish/client.go                  raw Redfish HTTP client (session auth, TLS, JSON decode)
  redfish/collector.go               (stub) planned wiring of client → metrics
  redfish/definition.go              (stub) planned metric name/label declarations
  shutdown/                          graceful shutdown helper
pkg/
  log/                               structured logger built on zap
  version/                           version metadata set by ldflags at build time
k8s_configuration/
  chart/                             Helm chart for Kubernetes deployment
  plugindefinition.yaml              Greenhouse plugin definition
```

### Configuration and live reload

Config is read from `.rootstock.yaml` (viper) **and** a separate
`redfish_pdus.yaml` file watched by `InitFileConfig` (fsnotify). When
`redfish_pdus.yaml` changes at runtime, the server is torn down and restarted
with the new credentials map — no restart of the binary is needed. Config
precedence (lowest to highest): defaults → config file → env vars → CLI flags.

### Credentials format (`redfish_pdus.yaml`)

```yaml
pdu1:
  username: redfish
  password: secret
pdu2:
  username: admin
  password: other
```

The PDU ID key (e.g. `pdu1`) must match the `labels` query parameter on probe
requests.

### TLS

PDUs ship with self-signed certs; the Redfish client intentionally skips
certificate verification (`InsecureSkipVerify: true`). This is expected and
linted with `//nolint:gosec`.

## CI / release pipeline

- **CI** runs on every PR: Go tests, golangci-lint, and REUSE compliance.
- **CodeQL** ([.github/workflows/codeql.yaml](.github/workflows/codeql.yaml))
  runs real Go security scanning via the shared reusable workflow
  `cloudoperators/common/.github/workflows/shared-codeql.yaml@main` (with
  `autobuild: true`, Go 1.27). It triggers on push to `main`, on PRs
  (opened/synchronize/reopened), and on a weekly schedule (Mondays 08:20 UTC).
  Code scanning alerts auto-close once a post-merge analysis on `main` no longer
  detects the issue.
- **Automerge**
  ([.github/workflows/automerge.yaml](.github/workflows/automerge.yaml))
  triggers when CI + CodeQL + REUSE all pass **and** a PR has an approved
  review. It updates `CHANGELOG.md` + `VERSION.yaml` via `git-cliff --bump`,
  then squash-merges the PR.
- PRs blocked by labels `invalid`, `WIP`, `wip`, `do-not-merge`, `DO-NOT-MERGE`
  are excluded from automerge.
- Docker images: `pr-<n>` tags for PRs (cleaned up on merge), versioned +
  `latest` tags for releases.

## Integration tests

Integration tests require the DMTF Redfish mockup server and are tagged
`//go:build integration`. `make test-integration` handles starting/stopping the
container via `hack/mockup-start.sh`. The PDU mockup data lives in
`testdata/pdu-mockup/`.

## Go test conventions

Tests must use `github.com/stretchr/testify/assert` (and `require` where
appropriate) for assertions.

Tests must be written as **table-driven tests**. Each entry in the table is a
struct with:

- `name` — label passed to `t.Run`
- input fields — values fed into the function under test
- state modifier fields — setup functions or values that establish
  pre-conditions (e.g. `fileContent string`, `monkeyPatchFunctionReturn error`)
- expected output fields — the values or behavior the function should produce
  (conventionally prefixed `want`)

See [cmd/mountfile_test.go](cmd/mountfile_test.go) for a canonical example.

## Shell script conventions

### Shared helpers (`hack/_helper.sh`)

All scripts under `hack/` must **source** [hack/_helper.sh](hack/_helper.sh)
rather than redefining the shared logging, `die`, and command-runner helpers
inline. Source it near the top, right after `set -euo pipefail`:

```sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../_helper.sh"
```

Adjust the relative path to reach `hack/_helper.sh` from the script's location
(e.g. `../_helper.sh` for scripts one level deep like `hack/kind/`). The file is
meant to be sourced, not executed, and guards against repeated sourcing, so
scripts that source each other will not redefine anything.

Sourcing `_helper.sh` provides:

- The color constants (`HC`, `FMAG`, `GREEN`, `YELLOW`, `RED`, `NC`).
- The logging helpers `log_debug` / `log_info` / `log_warn` / `log_error`.
- `die` — logs an error via `log_error` and exits non-zero.
- The command runners `_exec` and `_exec_output` (see below).

Do **not** re-declare any of these in individual scripts; rely on the sourced
versions so behavior stays consistent.

### Logging

The log helpers provided by `_helper.sh` are:

```sh
log_debug() { if [ "${DEBUG:-false}" = "true" ]; then printf "${HC}${FMAG}[.]${NC} %s\n" "$*" >&2; fi; }
log_info()  { printf "${GREEN}[+]${NC} %s\n" "$*" >&2; }
log_warn()  { printf "${YELLOW}[!]${NC} %s\n" "$*" >&2; }
log_error() { printf "${RED}[error]${NC} %s\n" "$*" >&2; }
```

- `log_debug` — bold-magenta `[.]` prefix; no-ops unless `DEBUG=true`; always
  writes to stderr.
- `log_info` — bold-green `[+]` prefix; always prints; always writes to stderr.
- `log_warn` — bold-yellow `[!]` prefix; always prints; always writes to stderr.
- `log_error` — bold-red `[error]` prefix; always prints; always writes to
  stderr.

Never use bare `echo` for diagnostic or status output; use these helpers
exclusively.

### Command runners

Use `_exec` / `_exec_output` from `_helper.sh` to run external commands so
failures are reported consistently (the failed command is printed in red and its
exit status propagated). When `DEBUG=true`, `_exec` also logs the command before
running it.

#### `_exec cmd [args...]`

Runs a command, reporting failures consistently.

- **Arguments:** the command and its arguments. With **multiple** arguments the
  command is executed directly (no shell, verbatim args — no word-splitting or
  glob expansion). With a **single** argument the string is run through
  `bash -c`, enabling pipes, redirects, and command substitution.
- **Behavior:** logs `exec: <cmd>` via `log_debug` when `DEBUG=true`. On a
  non-zero exit, logs the failed command in red via `log_error`.
- **Returns:** the command's exit status (non-zero on failure).

```sh
_exec kubectl apply -f "$file"                      # direct, verbatim args
_exec "kubectl get pods | grep rootstock >/dev/null"  # single string → bash -c
```

#### `_exec_output VAR cmd [args...]`

Runs a command via `_exec` and captures its stdout.

- **Arguments:** `VAR` — the name of the variable to assign stdout to; followed
  by the command and its arguments. The single-argument `bash -c` form applies
  to the command portion too.
- **Behavior:** on success, assigns the command's stdout to `VAR` (via
  `printf -v`). On failure, `VAR` is left unset and the error is reported by
  `_exec`.
- **Returns:** the command's exit status (non-zero on failure).

```sh
_exec_output NS "echo '$doc' | yq -r '.metadata.namespace'"
echo "namespace is $NS"
```

### Structure

Scripts must follow this structure:

1. **`main()` at the top** — high-level orchestration only; reads/validates
   parameters, then calls named helper functions in order. No inline logic.
2. **Helper functions below `main()`** — one function per distinct step or
   concern. Names must describe what they do.
3. **`main "$@"` as the last line** — the only call outside a function body.

```sh
#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../_helper.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

One-line description of what this script does.

Options:
  --foo VALUE    Description of foo (env: SCRIPT_FOO, default: bar)
  --help         Show this help and exit
EOF
}

main() {
  local foo="${SCRIPT_FOO:-bar}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --foo)    foo="$2"; shift 2 ;;
      --help)   usage; exit 0 ;;
      *)        printf "Unknown option: %s\n" "$1" >&2; usage; exit 1 ;;
    esac
  done

  step_one "$foo"
  step_two
}

step_one() {
  local foo="$1"
  log_info "Running step one with foo=${foo}"
  # ...
}

step_two() {
  log_info "Running step two"
  # ...
}

main "$@"
```

Rules:

- Every environment variable the script reads must also be settable via a
  corresponding `--flag` CLI parameter.
- The `--help` flag must print a usage block that names every flag and its
  corresponding env var.
- Default values belong in `main()`, not scattered across helper functions.
- Lines must not exceed 120 characters. Break long strings by building them in a
  local variable across multiple lines before use.

## REUSE / licensing

Every new source file must carry an SPDX header:

```go
// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0
```

CI validates this with the REUSE tool. All contributions require a DCO sign-off.
