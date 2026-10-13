<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
SPDX-License-Identifier: Apache-2.0
-->

# Greenhouse PluginDefinition verifier

`verify-plugindefinition.sh` proves that the rootstock
[`PluginDefinition`](../../k8s_configuration/plugindefinition.yaml) is valid
against a *real* Greenhouse control plane — without assuming any pre-existing
Greenhouse checkout.

It bootstraps Greenhouse-in-kind from scratch, publishes the rootstock Helm
chart to Greenhouse's local OCI registry, applies the `PluginDefinition`, and
asserts it reconciles to `READY=True`.

## Usage

```bash
# Fast path: bring up Greenhouse, publish the chart, assert PluginDefinition READY.
hack/greenhouse/verify-plugindefinition.sh

# Full end-to-end: also deploy a Plugin to the remote cluster and probe metrics.
hack/greenhouse/verify-plugindefinition.sh --with-plugin

# Keep clusters + clone around afterwards for debugging.
hack/greenhouse/verify-plugindefinition.sh --keep
```

## What it does

| Phase | Action |
|-------|--------|
| 0 | Preflight: require `docker kind kubectl helm yq go git jq` + a running docker daemon. |
| 1 | `git clone --depth 1 --branch $GREENHOUSE_REF` of cloudoperators/greenhouse into `$WORKDIR` (git-ignored). |
| 2 | `make setup` in the clone: creates kind clusters `greenhouse-admin` + `greenhouse-remote`, cert-manager, Flux, the manager, the local zot registry, and the `demo` Organization. |
| 3 | `helm package --version $CHART_VERSION` the chart and `helm push` it to `oci://localhost:5000/charts` as `rootstock:$CHART_VERSION`. |
| 4 | Rewrite `helmChart.repository` → `oci://registry:5000/charts` **and** `spec.version` / `helmChart.version` → `$CHART_VERSION` in a **temp copy**, `kubectl apply` the `PluginDefinition` into the `demo` org namespace, and poll for `READY=True`. **This is the primary pass/fail signal.** |
| 5 | *(only with `--with-plugin`)* `kind load` the rootstock image into the remote cluster, create the Redfish `Secret`, apply a `Plugin`, assert `Ready`, then port-forward and verify `/healthz` + `/probe` emit real metrics. |
| 6 | Teardown (skipped with `--keep`): delete both kind clusters, the `registry` container, and `$WORKDIR`. |

The committed `plugindefinition.yaml` is **never mutated** — all rewrites happen
on temp copies under a `mktemp` directory.

## Pinned Greenhouse ref

The script pins Greenhouse to a fixed, known-good tag for reproducibility:

```
GREENHOUSE_REF=v0.17.2   # default
```

Bump it deliberately when you want to track a newer Greenhouse release, e.g.:

```bash
GREENHOUSE_REF=v0.18.0 hack/greenhouse/verify-plugindefinition.sh
```

## Chart version

The chart version is read **dynamically** so the verifier always tests whatever
the repository currently declares — nothing is hardcoded:

- **Chart version** (packaged, pushed, and requested by the `PluginDefinition`)
  comes from [`k8s_configuration/chart/Chart.yaml`](../../k8s_configuration/chart/Chart.yaml)
  `.version`. This is the central authority for what gets tested.
- **App/image version** (the `--with-plugin` image tag) comes from
  [`VERSION.yaml`](../../VERSION.yaml) `.version`, matching the Makefile's
  single-source-of-truth convention.

To test a different chart version — e.g. an older release — override
`CHART_VERSION`:

```bash
CHART_VERSION=0.0.9 hack/greenhouse/verify-plugindefinition.sh
```

The script packages with `helm package --version "$CHART_VERSION"` (which does
**not** mutate `Chart.yaml`) and rewrites the temp `PluginDefinition` to match,
so the chart that is pushed and the version Greenhouse pulls always agree.

> Note: overriding `CHART_VERSION` repackages the **current** chart source under
> that version number. It does not check out the historical chart templates; to
> reproduce an old chart faithfully, check out the matching git tag first.

## Environment overrides

| Variable | Default | Purpose |
|----------|---------|---------|
| `GREENHOUSE_REF` | `v0.17.2` | Greenhouse git tag/commit to clone. |
| `GREENHOUSE_REPO` | `https://github.com/cloudoperators/greenhouse` | Source repo. |
| `WORKDIR` | `<repo>/.greenhouse-verify` | Where the clone + kubeconfigs live. |
| `ADMIN_CLUSTER` / `REMOTE_CLUSTER` | `greenhouse-admin` / `greenhouse-remote` | kind cluster names (match the pinned Makefile). |
| `DEMO_ORG` | `demo` | Greenhouse Organization namespace. |
| `REGISTRY_HOST` | `localhost:5000` | Registry address reachable from the host (`helm push`). |
| `REGISTRY_INCLUSTER` | `registry:5000` | Registry address reachable from kind nodes / Flux. |
| `REGISTRY_CHART_REPO` | `charts` | OCI path the chart is pushed under. |
| `CHART_VERSION` | `Chart.yaml .version` | Chart version to package/push/test (override to test another version). |
| `APP_VERSION` | `VERSION.yaml .version` | App/image tag version. |
| `ROOTSTOCK_IMAGE` | `rootstock:<APP_VERSION>` | Image loaded into the remote cluster for `--with-plugin`. |
| `READY_TIMEOUT` | `300` | Seconds to wait for `READY=True`. |
| `REDFISH_PASSWORD` | `redfish-test-password` | Password put in the Redfish `Secret` (`--with-plugin`). |

## Caveats

- `make setup` is heavy and can be flaky on the first run (image pulls, cert
  issuance, Flux reconciliation). The script fails loudly; re-run on transient
  failures.
- **Host port 5000 must be free.** Greenhouse's `hack/registry/setup.sh`
  hard-codes the local registry on port 5000. On macOS this collides with the
  AirPlay Receiver (ControlCenter). The script detects this in preflight and
  tells you to disable it (System Settings → General → AirDrop & Handoff →
  AirPlay Receiver). A stale `registry` container from a previous run is removed
  automatically.
- `--with-plugin` requires the `rootstock` image to exist locally
  (`docker image inspect rootstock:<APP_VERSION>`, where `APP_VERSION` is read
  from `VERSION.yaml`). Build it first with `make build` / `make kind-up`.
- Cluster/org names and the registry addresses are the pinned-ref defaults; if
  you point at a different `GREENHOUSE_REF` whose Makefile differs, override the
  matching variables above.
