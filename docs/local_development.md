# Local development

This document describes how to get the local rootstock setup to work.

- [Local development](#local-development)
  - [Prerequisites](#prerequisites)
    - [golangci-lint installation](#golangci-lint-installation)
    - [prettier installation](#prettier-installation)
  - [Running linters](#running-linters)
  - [Cobra CLI](#cobra-cli)
  - [Configuration inputs](#configuration-inputs)
  - [1-time config preparation](#1-time-config-preparation)
  - [run rootstock locally](#run-rootstock-locally)

## Prerequisites

- [Go](https://golang.org/dl/) 1.26+
- [golangci-lint](https://golangci-lint.run/) for Go linting
- [prettier](https://prettier.io/docs/install) for markdown linting

### golangci-lint installation

Install **golangci-lint v2.13.2** — this is the exact version CI runs (pinned in
`.github/workflows/ci.yaml`). Using the same version locally guarantees your
results match CI; a different version can report different findings against the
same config.

The recommended way is via the official install script:

```shell
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sh -s -- -b $(go env GOPATH)/bin v2.13.2
```

Homebrew installs the latest release, which may differ from the pinned version,
so prefer the script above. If you do use Homebrew, check the version
afterwards:

```shell
brew install golangci-lint
```

Verify you have the pinned version:

```shell
golangci-lint --version   # should report 2.13.2
```

### prettier installation

For the installation of `prettier` run

```shell
brew install prettier
```

Alternatively, you can use the
[installation documnentation](https://prettier.io/docs/install).

## Running linters

To run linters and tidy the `go.mod` file, run

```bash
make lint
```

This runs golangci-lint against the full codebase using the configuration in
`.golangci.yaml` at the repo root.

The `conventions` job in CI runs the same linting on every pull request against
`main`.

## Cobra CLI

Rootstock is a [cobra cli](https://github.com/spf13/cobra). That means that its
API with commands and flags can be explored using the cli-parameter `-h` or
`--help` like

```shell
rootstock -h

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  run         run the rootstock server
  version     Returns the cli version

Flags:
      --add-trace              if this flag is set, error logs will have a stacktrace appended. (env: "ADD_TRACE")
      --config string          absolute config path (env: "CONFIG_PATH") (default "/config/.rootstock.yaml")
      --config-folder string   folder where the config files are stored (env: "CONFIG_FOLDER") (default "/config")
  -h, --help                   help for rootstock
  -f, --log-format string      if this flag is set, logs will be presented in text format - options: text, json (env: "LOG_FORMAT") (default "json")
  -l, --log-lvl string         application log level options: key or value of [debug info warn error] (env: "LOG_LEVEL") (default "info")
      --tmp-folder string      tmp folder used by rootstock (env: "TMP_FOLDER")

Use "rootstock [command] --help" for more information about a command.
```

## Configuration inputs

Rootstock accepts configuration input via 4 routes, ordered from least to
highest precedence:

- `defaults`
- `config-file`
- `environment variable`
- `commandline flag`

## 1-time config preparation

To start rootstock, prepare the following config files by running

```shell
mkdir -p tmp

echo "example_pw" > tmp/redfish_password

cat <<EOF >>tmp/.rootstock.yaml
rootstock:
  log:
    level: debug
    format: text
    # trace: true
  redfish:
    credentialsFile: tmp/redfish_password
  config:
    folder: .
EOF
```

## run rootstock locally

To run rootstock locally, make sure that the
[1-time setup](#1-time-config-preparation) is complete. Then execute

```shell
go run main.go run --config ./tmp/.rootstock.yaml
```

Alternatively, you can also forego a config file by directly running

```shell
go run main.go run \
  --redfish-credentials-file tmp/redfish_password \
  --config-folder . \
  --log-format text
```
