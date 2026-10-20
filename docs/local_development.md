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
  - [Changelog update](#changelog-update)
  - [Docker images](#docker-images)
    - [Local builds](#local-builds)
    - [PR builds](#pr-builds)
    - [Release builds](#release-builds)

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

CI runs golangci-lint on every pull request against `main`; Markdown formatting
and `go mod tidy` are currently only run by the local `make lint` target.

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

## Changelog update

The changelog is generated from git history using
[git-cliff](https://git-cliff.org/). CI runs the update automatically on every
merge to `main`; to preview or update it locally, install `git-cliff` first:

```shell
brew install git-cliff
```

Then regenerate the changelog:

```shell
make generate-changelog
```

This rewrites `CHANGELOG.md` in full from a fixed start commit up to the current
tip of `main`. All commits that are not yet behind a release tag are grouped
under the computed next version (using `--bump`), so the file always reflects
what the next release will look like.

An additional "commit" can be added to the changelog, by setting the environment
variable `export COMMIT_MSG="..."`.

To also bump `VERSION.yaml` to match:

```shell
make generate-version
```

Both targets are run together by the automerge pipeline, but running them
locally is useful for reviewing the changelog before opening a PR or after
rebasing onto `main`.

To run the changelog generation test suite:

```shell
make test-changelog
```

## Docker images

For a proper release, the rootstock binary is put into a docker container. The
container is designed to be a multi-architecture container and can be consumed
by running

```shell
image=ghcr.io/cloudoperators/rootstock:latest
docker run -it --platform linux/amd64 "${image}" version.   # for a specified platform
docker run -it "${image}" version.                          # for the system platform
```

### Local builds

You can build the rootstock docker container locally by running either of these
commands.

```shell
make docker-build                         # builds for your local architecture
make docker-build PLATFORM=linux/amd64    # builds for the specified platform
```

This will result in a local docker image in the format
`ghcr.io/cloudoperators/rootstock:dev-${D/I-number}`.

### PR builds

During pull-requests, the docker image is built in the ci-validation pipeline
and released to the github container registry under `ghcr.io/cloudoperators`.
For pull-requests, the image is tagged as
`ghcr.io/cloudoperators/rootstock:pr-${pr-number}`. PR images are cleaned up
periodically and after PRs are merged or closed. However, these images can be
pulled for investigative purposes by connecting to `ghcr.io` and pulling the
image as such:

```shell
# login
echo "${GITHUB_TOKEN}" | docker login ghcr.io -u "${GITHUB_USERNAME}" --password-stdin
# pull image
pr_number=78
docker pull ghcr.io/cloudoperators/rootstock:pr-${pr_number}
```

### Release builds

For releases, similarly as for [PR builds](#pr-builds), the rootstock image is
created in a pipeline. The only difference is, that this time images are not
cleaned up and each image receives more tags for pulling convenience:

```shell
ghcr.io/cloudoperators/rootstock:${version}         # e.g. 1.1.0
ghcr.io/cloudoperators/rootstock:${major}           # e.g. v1
ghcr.io/cloudoperators/rootstock:${git_hash}        # e.g. a0e012eae7d95f9d961d27e521e84b3493c19190
ghcr.io/cloudoperators/rootstock:${git_hash_short}  # e.g. a0e012e
ghcr.io/cloudoperators/rootstock:latest
```
