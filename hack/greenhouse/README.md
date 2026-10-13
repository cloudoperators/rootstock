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

## Quick verification

The ergonomic path — build the image, then run the full end-to-end. The longer
`READY_TIMEOUT` absorbs slow first-run image pulls:

```bash
cd <repo root>

# 1. Build the rootstock image (required for the full --with-plugin run).
make build

# 2. Bring up Greenhouse, publish the chart, deploy the Plugin + Redfish mockup,
#    and probe real metrics. A pass ends with "==> VERIFICATION PASSED" (exit 0).
READY_TIMEOUT=600 hack/greenhouse/verify-plugindefinition.sh --with-plugin
```

Add `--keep` to leave the clusters + clone up for inspection afterwards. With
warm clusters the script auto-busts the Flux chart cache, so re-running is safe.

### Inspect the evidence

With a `--keep` run still up, confirm status, the live workload, and real probe
output in one shot:

```bash
kubectl --context kind-greenhouse-admin -n demo get plugindefinition,plugin && \
kubectl --context kind-greenhouse-remote -n rootstock get pods && \
( kubectl --context kind-greenhouse-remote -n rootstock port-forward svc/rootstock 9220:9220 >/dev/null 2>&1 & \
  PF=$!; sleep 2; \
  echo "--- /healthz ---"; curl -s localhost:9220/healthz; \
  echo "--- /probe ---";   curl -s "localhost:9220/probe?target=http://redfish-mockup.rootstock.svc.cluster.local:8000" | head -20; \
  kill "$PF" )
```

Expect the Plugin at `Ready=True`, `rootstock` + `redfish-mockup` pods
`Running`, `/healthz` OK, and `/probe` emitting metrics.

## Invocation modes

```bash
# Chart-only: bring up Greenhouse, publish the chart, assert PluginDefinition READY.
hack/greenhouse/verify-plugindefinition.sh

# Full end-to-end: also deploy a Plugin to the remote cluster and probe metrics.
hack/greenhouse/verify-plugindefinition.sh --with-plugin

# Keep clusters + clone around afterwards for debugging.
hack/greenhouse/verify-plugindefinition.sh --with-plugin --keep
```

The default (chart-only) run does **not** need the local image; `--with-plugin`
does.

## What it does

The script runs these phases in order:

0. **Preflight** — verifies the required tools and a running Docker daemon are
   present, failing fast if anything is missing.
1. **Clone Greenhouse** — shallow-clones the pinned Greenhouse ref into the
   git-ignored work directory.
2. **Bootstrap** — stands up the two kind clusters (admin + remote) along with
   cert-manager, Flux, the Greenhouse manager, the local zot registry, and the
   demo Organization.
3. **Publish the chart** — packages the rootstock chart at the resolved version
   and pushes it to the local OCI registry, then invalidates any stale Flux
   chart cache so warm clusters re-pull the fresh artifact.
4. **Apply the PluginDefinition** — repoints a temp copy at the in-cluster
   registry and version, applies it to the demo org, and waits for it to
   reconcile. This readiness check is the primary pass/fail signal.
5. **Full E2E** *(only with `--with-plugin`)* — pushes the rootstock and
   test-connection images to the registry as OCI artifacts, deploys the Redfish
   mockup (unless a target is supplied), and applies a normalized Plugin. It
   then asserts the Plugin is Ready and verifies the health and probe endpoints
   emit real metrics.
6. **Teardown** *(skipped with `--keep`)* — deletes both kind clusters, the
   registry container, and the work directory.

The committed `plugindefinition.yaml` and `plugin.yaml` are **never mutated** —
all rewrites happen on temp copies under a `mktemp` directory.

### Why images are pushed to the registry (not `kind load`ed)

Greenhouse resolves the images referenced by a rendered HelmRelease from an OCI
registry, so loading them into node containerd with `pullPolicy: Never` is not
enough. The script therefore pushes to zot via a throwaway `docker-container`
buildx builder on the host network with `registry.insecure=true`, because zot
only accepts **OCI** manifests (a plain `docker push` emits Docker v2 and is
rejected with `manifest invalid`). Image refs use the plain `registry:5000/...`
path on purpose: Greenhouse rewrites images on known upstream domains
(`docker.io`, `ghcr.io`, `gcr.io`, `quay.io`, `registry.k8s.io`) to its
on-demand mirror, but a `registry:5000/...` ref has a `:port` and is treated as
a real registry, so it passes through un-rewritten.

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
| `ROOTSTOCK_IMAGE` | `rootstock:<APP_VERSION>` | Local build tag (source image) pushed to the registry for `--with-plugin`. |
| `ROOTSTOCK_REGISTRY_IMAGE` | `registry:5000/rootstock:<APP_VERSION>` | In-cluster ref the Plugin/HelmRelease pulls from. |
| `TESTCONN_SOURCE_IMAGE` | `curlimages/curl:8.11.1` | Upstream curl image pulled once and re-published for the chart's Helm test hook. |
| `TESTCONN_REGISTRY_IMAGE` | `registry:5000/curl:<tag>` | In-cluster ref the test-connection pod pulls from. |
| `REMOTE_ONBOARDED_NAME` | `kind-<REMOTE_CLUSTER>` | Name the remote cluster is onboarded as; a Plugin's `spec.clusterName`. |
| `TARGET` | bundled DMTF mockup svc | Redfish endpoint probed under `--with-plugin`; when set, the mockup is NOT deployed. |
| `READY_TIMEOUT` | `300` | Seconds to wait for `READY=True`. |
| `POLL_INTERVAL` | `5` | Seconds between readiness polls. |
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
