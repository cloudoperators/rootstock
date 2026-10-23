<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
SPDX-License-Identifier: Apache-2.0
-->

# rootstock Helm chart

A Helm chart for [rootstock](https://github.com/cloudoperators/rootstock), a
data-center energy/carbon telemetry collector that scrapes Redfish targets and
exposes Prometheus metrics using a multi-target probe pattern.

The chart is a standard `application` chart and is compatible with
[Greenhouse](https://github.com/cloudoperators/greenhouse) plugins (see
`k8s_configuration/plugindefinition.yaml`).

## Local testing on kind

From the repository root, the Makefile wires up a full local environment (builds
the image, deploys an in-cluster DMTF Redfish mockup pre-loaded with
`testdata/pdu-mockup`, creates the credentials Secret, and installs the chart):

```bash
make kind-up        # create cluster + build/load image + mockup + deploy chart
make kind-validate  # port-forward and probe the running pod
make kind-delete    # delete the cluster
```

`helm test rootstock -n rootstock` runs an in-cluster smoke test against
`/healthz` and `/ready`.

**alternatively**

```bash
# Self-contained (inline per-PDU credentials, chart renders the Secret):
helm install rootstock k8s_configuration/chart \
  --set 'rootstock.redfish.access.pdu1.username=redfish' \
  --set 'rootstock.redfish.access.pdu1.password=s3cr3t'

# Production style (reference a pre-created Secret):
kubectl create secret generic rootstock-redfish \
  --from-file=redfish_pdus.yaml=./redfish_pdus.yaml
helm install rootstock k8s_configuration/chart \
  --set rootstock.redfish.existingSecret=rootstock-redfish
```

## Secrets: Redfish credentials

rootstock reads per-PDU credentials from a mounted YAML file at
`<rootstock.config.folder>/redfish_pdus.yaml`. The file is watched (fsnotify)
and hot-reloaded on change. The chart always projects it as a **read-only
mounted file** and never passes credentials as environment variables.

There are two mutually exclusive ways to provide credentials:

| Mode                | Value                                      | Use case                                                       |
| ------------------- | ------------------------------------------ | -------------------------------------------------------------- |
| **Existing Secret** | `rootstock.redfish.existingSecret: <name>` | Production; Secret managed outside the chart.                  |
| **Inline**          | `rootstock.redfish.access: {pdu1: {...}}`  | Self-contained / local (kind) runs; the chart renders a Secret.|

In both modes the Secret must contain the key `redfish_pdus.yaml` in this
format:

```yaml
pdu1:
  username: redfish
  password: s3cr3t
```

### Greenhouse

Use `rootstock.redfish.existingSecret` and pre-create the Secret in the
namespace before installing the Plugin, or set `rootstock.redfish.access`
directly via `optionValues` if the Plugin operator supports map values.

## Values

See [`values.yaml`](./values.yaml) for the full list and inline documentation.
Key options:

| Key                                  | Default                            | Description                                   |
| ------------------------------------ | ---------------------------------- | --------------------------------------------- |
| `image.repository`                   | `ghcr.io/cloudoperators/rootstock` | Image repository.                             |
| `image.tag`                          | `""` (chart appVersion)            | Image tag.                                    |
| `rootstock.redfish.username`         | `redfish`                          | Redfish auth username.                        |
| `rootstock.redfish.existingSecret`   | `""`                               | Pre-created Secret holding credentials.       |
| `rootstock.redfish.access`           | `{pdu1: {username: redfish, ...}}` | Inline per-PDU credentials (renders a Secret).|
| `rootstock.config.folder`            | `/config`                          | Mount directory for config and credentials.   |
| `rootstock.port`                     | `9220`                             | Container port.                               |
| `rootstock.scrape.timeout`           | `30s`                              | Upstream Redfish scrape timeout.              |
| `service.type` / `service.port`      | `ClusterIP` / `9220`               | Service exposure.                             |
| `serviceMonitor.enabled`             | `false`                            | Create a Prometheus Operator ServiceMonitor.  |

## Endpoints

rootstock serves `/healthz`, `/ready`, `/metrics`, and the multi-target
`/probe?target=<redfish-url>&labels=<pdu_id>` on `rootstock.port`.
