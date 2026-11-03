<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
SPDX-License-Identifier: Apache-2.0
-->

# Greenhouse local dev environment

Two scripts manage a local Greenhouse-in-kind environment for rootstock:

| Script                       | Purpose                                                                          |
| ---------------------------- | -------------------------------------------------------------------------------- |
| `control.sh`                 | Lifecycle: create or delete the Greenhouse dev environment                       |
| `verify_plugindefinition.sh` | Verification: publish the chart, apply the PluginDefinition, assert `READY=True` |

Makefile targets wrap both scripts:

```bash
make greenhouse-create  # control.sh create
make greenhouse-verify  # verify_plugindefinition.sh
make greenhouse-delete  # control.sh delete
```

## Quick start

```bash
# 1. Bootstrap Greenhouse-in-kind (admin + remote clusters, zot registry, demo org).
make greenhouse-create

# 2. Verify the PluginDefinition (chart-only, no image needed).
make greenhouse-verify

# 3. Full end-to-end — also deploy a Plugin + Redfish mockup and probe real metrics.
#    Build the rootstock image first.
make docker-build
hack/greenhouse/verify_plugindefinition.sh --with-plugin

# 4. Tear down when done.
make greenhouse-delete
```

## control.sh

Bootstraps or tears down the local Greenhouse dev environment.

```text
Usage: control.sh <command> [OPTIONS]

Commands:
  create    Bootstrap a local Greenhouse-in-kind environment (clone + make setup).
  delete    Delete the kind clusters, registry container, and workdir.
```

### create

Runs these steps in order:

1. **Preflight** — checks that `docker`, `kind`, `kubectl`, `git`, `make` are
   available and the Docker daemon is reachable.
2. **Port check** — ensures host port 5000 is free for the local registry. A
   stale `registry` container from a previous run is removed automatically. On
   macOS, AirPlay Receiver occupies port 5000 by default; the script detects
   this and prints instructions to disable it.
3. **Fetch Greenhouse** — shallow-clones the pinned ref into
   `tmp/greenhouse/setup/`. Reuses an existing clone if it is already at the
   right ref.
4. **Bootstrap** — runs `make setup-manager setup-demo` inside the Greenhouse
   clone, bringing up the admin + remote kind clusters, cert-manager, Flux, the
   Greenhouse manager, the local zot registry, and the demo Organization. The
   `setup-dashboard` target is skipped — it is not needed for PluginDefinition
   verification and is flaky locally.

```bash
# Override the pinned Greenhouse version:
GREENHOUSE_REF=v0.18.0 hack/greenhouse/control.sh create

# Override the demo org namespace:
DEMO_ORG=myorg hack/greenhouse/control.sh create
```

### delete

Deletes both kind clusters, the `registry` Docker container, and the
`tmp/greenhouse/setup/` workdir.

```bash
hack/greenhouse/control.sh delete
```

## verify_plugindefinition.sh

Verifies the rootstock
[`PluginDefinition`](../../k8s_configuration/plugindefinition.yaml) against a
running Greenhouse dev environment. Run `control.sh create` first.

```text
Usage: verify_plugindefinition.sh [--with-plugin]
```

Runs these phases in order:

0. **Preflight** — checks required tools (`docker`, `kind`, `kubectl`, `helm`,
   `yq`, `go`, `git`, `jq`), the local registry container, and the source files.
1. **Publish chart** — packages the rootstock chart at the resolved version and
   pushes it to the local OCI registry (`oci://localhost:5000/charts`).
   Invalidates any stale Flux HelmChart cache so re-runs on warm clusters always
   pull the fresh artifact.
2. **Apply PluginDefinition** — rewrites a temp copy of `plugindefinition.yaml`
   to point at the in-cluster registry and pin the chart version, applies it to
   the demo org namespace, and waits for `READY=True`. This is the primary
   pass/fail signal.
3. **Full E2E** _(only with `--with-plugin`)_ — pushes the rootstock image and
   the test-connection curl image to the registry as OCI artifacts, creates the
   Redfish `Secret`, applies a normalized `Plugin`, asserts it becomes `Ready`,
   deploys the bundled DMTF Redfish mockup (unless `TARGET` is set), then
   port-forwards and asserts `/healthz` and `/probe` emit real metrics.

Teardown (workdir + OCI buildx builder) always runs on exit.

The committed `plugindefinition.yaml` and `plugin.yaml` are **never mutated** —
all rewrites happen on temp copies under a `mktemp` directory.

### Why images are pushed to the registry (not `kind load`ed)

Greenhouse resolves images referenced by a rendered HelmRelease from an OCI
registry, not from node containerd. The script therefore pushes to zot via a
throwaway `docker-container` buildx builder on the host network — zot only
accepts OCI manifests (plain `docker push` emits Docker v2 schema and is
rejected with `manifest invalid`). Image refs use the `registry:5000/...` path
because Greenhouse rewrites images on known upstream domains (`docker.io`,
`ghcr.io`, etc.) through its on-demand mirror; a `registry:5000/...` ref with a
port is treated as a plain registry and passes through un-rewritten.

## Environment overrides

### control.sh overrides

| Variable         | Default   | Purpose                             |
| ---------------- | --------- | ----------------------------------- |
| `GREENHOUSE_REF` | `v0.18.0` | Greenhouse git tag/commit to clone. |
| `DEMO_ORG`       | `demo`    | Greenhouse Organization namespace.  |

### verify_plugindefinition.sh overrides

| Variable                   | Default                               | Purpose                                                                              |
| -------------------------- | ------------------------------------- | ------------------------------------------------------------------------------------ |
| `DEMO_ORG`                 | `demo`                                | Greenhouse Organization namespace.                                                   |
| `REGISTRY_HOST`            | `localhost:5000`                      | Registry address reachable from the host (`helm push`).                              |
| `REGISTRY_INCLUSTER`       | `registry:5000`                       | Registry address reachable from kind nodes / Flux.                                   |
| `REGISTRY_CHART_REPO`      | `charts`                              | OCI path the chart is pushed under.                                                  |
| `CHART_VERSION`            | `Chart.yaml .version`                 | Chart version to package/push/test.                                                  |
| `APP_VERSION`              | `VERSION.yaml .version`               | App/image tag version.                                                               |
| `ROOTSTOCK_IMAGE`          | `rootstock:<APP_VERSION>`             | Local build tag pushed to the registry for `--with-plugin`.                          |
| `ROOTSTOCK_REGISTRY_IMAGE` | `registry:5000/rootstock:<tag>`       | In-cluster ref the Plugin/HelmRelease pulls from.                                    |
| `TESTCONN_SOURCE_IMAGE`    | `curlimages/curl:8.11.1`              | Upstream curl image re-published for the chart's Helm test hook.                     |
| `TESTCONN_REGISTRY_IMAGE`  | `registry:5000/curlimages/curl:<tag>` | In-cluster ref the test-connection pod pulls from.                                   |
| `TARGET`                   | bundled DMTF mockup svc               | Redfish endpoint probed under `--with-plugin`; when set, the mockup is not deployed. |
| `READY_TIMEOUT`            | `60`                                  | Seconds to wait for `READY=True`.                                                    |
| `POLL_INTERVAL`            | `5`                                   | Seconds between readiness polls.                                                     |
| `REDFISH_PASSWORD`         | `redfish-test-password`               | Password written to the Redfish `Secret` (`--with-plugin`).                          |

## Caveats

- **`control.sh create` is slow and can be flaky on first run** — image pulls,
  cert issuance, and Flux reconciliation all take time. The script fails loudly
  on error; re-run on transient failures.
- **Host port 5000 must be free.** Greenhouse hard-codes the local registry on
  port 5000. On macOS this collides with AirPlay Receiver; the `create` command
  detects this and prints instructions to disable it.
- **`--with-plugin` requires the rootstock image to exist locally.** Build it
  first with `make docker-build`. The image tag is derived from `VERSION.yaml`
  unless `ROOTSTOCK_IMAGE` is set.
- **`CHART_VERSION` override repackages the current chart source**, not a
  historical checkout. To reproduce an old chart faithfully, check out the
  matching git tag first.
