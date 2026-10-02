# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors

# SPDX-License-Identifier: Apache-2.0

# Local Development

This guide covers tooling needed for local development on rootstock.

## Prerequisites

- [Go](https://golang.org/dl/) 1.26+
- [golangci-lint](https://golangci-lint.run/) for Go linting

## Installing golangci-lint

Install **golangci-lint v2.13.2** — this is the exact version CI runs (pinned in
`.github/workflows/ci.yaml`). Using the same version locally guarantees your
results match CI; a different version can report different findings against the
same config.

The recommended way is via the official install script:

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sh -s -- -b $(go env GOPATH)/bin v2.13.2
```

Homebrew installs the latest release, which may differ from the pinned version,
so prefer the script above. If you do use Homebrew, check the version
afterwards:

```bash
brew install golangci-lint
```

Verify you have the pinned version:

```bash
golangci-lint --version   # should report 2.13.2
```

## Running linters

Run Go linting locally before opening a PR:

```bash
make lint-go
```

This runs golangci-lint against the full codebase using the configuration in
`.golangci.yaml` at the repo root.

The `conventions` job in CI runs the same linting on every pull request against
`main`.
