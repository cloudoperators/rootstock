# Build and release setup

This document describes the general build, validation, and release setup of the
project.

## Table of contents

- [Build and release setup](#build-and-release-setup)
  - [Table of contents](#table-of-contents)
  - [Flow of changes in the repository](#flow-of-changes-in-the-repository)
  - [Pipeline steps](#pipeline-steps)
    - [Owner review](#owner-review)
    - [Convention checks](#convention-checks)
    - [Recurring scans](#recurring-scans)
    - [Build](#build)
    - [Tests](#tests)
      - [Unit tests](#unit-tests)
      - [Contract tests](#contract-tests)
      - [Integration tests](#integration-tests)
      - [Performance tests](#performance-tests)
    - [File generation](#file-generation)
    - [Release to greenhouse](#release-to-greenhouse)

## Flow of changes in the repository

To add a change to the rootstock repository, create a PR against the default
branch (`main`) of the repository. The change will then undergo the following
steps

1. pr-validation
   - [convention checks](#convention-checks)
   - [code scans](#recurring-scans)
   - [unit-tests](#unit-tests)
   - [build](#build)
   - [contract tests](#contract-tests)
2. [review](#owner-review)
3. merge-validation
   - [integration tests](#integration-tests)
4. release (if desired separately from merge to `main`)
   - [performance tests](#performance-tests)
   - [file generation](#file-generation)
   - [merge to release branch](#release-to-greenhouse)

These steps are automated via GitHub actions leveraging the
[common actions](https://github.com/cloudoperators/common/tree/main/.github/workflows)
from the GitHub organization where sensible.

## Pipeline steps

For change validation, we employ the following reusable steps.

### Owner review

All pull-requests must be reviewed by at least 1 codeowner.

### Convention checks

This project enforces formatting conventions for the most relevant file types.
The following linters and configuration files are used:

- `markdown`: [prettier](https://prettier.io/) -
  [.prettierrc.yaml](https://prettier.io/docs/options)
- `shell`: [shellcheck](https://github.com/koalaman/shellcheck) -
  [.shellcheckrc](https://github.com/koalaman/shellcheck/wiki/Ignore)
- `golang`: [golangci-lint](https://github.com/golangci/golangci-lint) -
  [.golangci.yaml](https://golangci-lint.run/docs/configuration/file/) ([TODO]
  use a cache e.g. like
  [example](https://github.tools.sap/AI/user-content/blob/main/.github/workflows/cache-build-component.yaml))

Additional conventions are:

- file names: concatenated filenames use dashes, e.g.
  (`my-concatenated-file-name.txt`)
- we follow the markdown style from
  [markdownlint](https://github.com/DavidAnson/markdownlint)
- markdown headlines start with a capital letter
- [TODO] define a single entrypoint for local repository interactions (e.g. a
  top-level `Makefile`).

### Recurring scans

- The
  [CodeQL workflow](https://github.com/cloudoperators/common/blob/main/.github/workflows/shared-codeql.yaml)
  from the `cloudoperators/common` repository implements vulnerability scanning
  for this repository.
- ([TODO] more required?)

### Build

- [TODO] check if we use
  https://github.com/cloudoperators/common/blob/main/.github/workflows/shared-go-build.yaml
- [TODO] use a cache e.g. like
  [example](https://github.tools.sap/AI/user-content/blob/main/.github/workflows/cache-build-component.yaml)

### Tests

For all tests, we use the standard go tooling. Unit tests come without a build
tag and all other test types have the name as respective build tag
test can be done by running

```shell
# unit tests
go test ./... -parallel=10 -timeout=10m

# integration tests
go test ./... -parallel=10 -timeout=10m -tags=integration
```

#### Unit tests

- [TODO] check
  [user-content](https://github.tools.sap/AI/user-content/.github/workflows/cache-build-component.yaml)
- [TODO] use a cache e.g. like
  [example](https://github.tools.sap/AI/user-content/blob/main/.github/workflows/cache-build-component.yaml)

#### Contract tests

Contract tests validate only the communication interface with external services,
instead of validating the fully integrated system with all external services
present, see e.g. [pact.io](https://docs.pact.io/) for a holistic presentation
of the topic.

In our pipelines we rely on contract tests ([TODO] do we? or do we prefer
explicit mocks?), to fix both

- the APIs that we expose
- as well as our expectation for upstream APIs.

#### Integration tests

Integration tests require the full system of dependent down-, and upstream
services to be present. This system can then be validated with end-to-end tests
that span the entire application flow. Since such tests are costly (in setup,
debugging, tear-down, brittleness), we restrict the number of these tests as
much as possible.

#### Performance tests

Performance tests are similar to unit or integration tests, but with a slightly
different scope. While the former validate general behaviour of the system,
performance tests explicitly investigate the performance boundaries the system
can handle. As such, these tests have typically longer runtimes and generate
more data throughput. We can use performance tests to ensure the
service-level-objectives (SLOs). Relevant data/metrics could be:

- queue length
  - each monitored server is requeued periodically for scraping consumption data
  - depending on the number of monitored servers, does the queue length
    increase?
  - can be done as a unit test
- concurrent API requests
  - how many concurrent requests can the rootstock endpoints handle without
    degradation?

In general, performance test results should be used to configure alerts and
load-shedding limits.

### File generation

If required, file generation will handle updating all generated files and
version fields.

([TODO] use a cache e.g. like
[example](https://github.tools.sap/AI/user-content/blob/main/.github/workflows/cache-build-component.yaml))

### Release to greenhouse

[TODO] how do we tie into greenhouse releases?

- does greenhouse connect to the `main` branch directly or to a dedicated
  release branch?
