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

## TL;DR

```bash
# Self-contained (inline password, chart renders the Secret):
helm install rootstock k8s_configuration/chart \
  --set redfish.password='s3cr3t'

# Production / Greenhouse style (reference a pre-created Secret):
kubectl create secret generic rootstock-redfish \
  --from-literal=redfish_password='s3cr3t'
helm install rootstock k8s_configuration/chart \
  --set redfish.existingSecret=rootstock-redfish
```

## Secrets: the Redfish password is read from a file, never an env var

rootstock reads the Redfish password **only** from a mounted file
(`--config-folder <dir>` + `--redfish-credentials-file <name>`, i.e.
`<configFolder>/<credentialsFileName>`, default `/config/redfish_password`).
The file is watched (fsnotify) and hot-reloaded on change. The chart therefore
always projects the password as a **read-only mounted file** and never sets it
as an environment variable.

There are two mutually exclusive ways to provide the password:

| Mode | Values | Use case |
| --- | --- | --- |
| **Existing Secret** | `redfish.existingSecret: <name>` | Production, Greenhouse (`valueFrom.secret`), CI with a pre-created Secret. Recommended. |
| **Inline** | `redfish.password: <value>` | Self-contained / local (kind) runs. The chart renders a Secret for you. |

In both modes the Secret is mounted at `redfish.configFolder` and the key
`redfish.secretKey` is projected to the file `redfish.credentialsFileName`
with mode `0400`.

### Greenhouse

In a `Plugin` CR, supply the password through `optionValues[].valueFrom.secret`:

```yaml
spec:
  optionValues:
    - name: redfish.username
      value: redfish
    - name: redfish.password
      valueFrom:
        secret:
          name: rootstock-redfish
          key: redfish_password
```

Greenhouse projects that into the pod as the mounted file the chart expects.

## Values

See [`values.yaml`](./values.yaml) for the full list and inline documentation.
Key options:

| Key | Default | Description |
| --- | --- | --- |
| `image.repository` | `ghcr.io/cloudoperators/rootstock` | Image repository. |
| `image.tag` | `""` (chart appVersion) | Image tag. |
| `redfish.username` | `redfish` | Redfish auth username. |
| `redfish.existingSecret` | `""` | Pre-created Secret holding the password. |
| `redfish.password` | `""` | Inline password (renders a Secret). |
| `redfish.secretKey` | `redfish_password` | Key within the Secret. |
| `redfish.configFolder` | `/config` | Mount dir (`--config-folder`). |
| `redfish.credentialsFileName` | `redfish_password` | File name (`--redfish-credentials-file`). |
| `server.port` | `9220` | REST server port. |
| `server.scrapeTimeout` | `30s` | Upstream scrape timeout. |
| `service.type` / `service.port` | `ClusterIP` / `9220` | Service exposure. |
| `serviceMonitor.enabled` | `false` | Create a Prometheus Operator ServiceMonitor. |

## Endpoints

rootstock serves `/healthz`, `/ready`, `/metrics`, and the multi-target
`/probe?target=<redfish-url>&labels=<pdu_id>` on `server.port`.

## Local testing on kind

From the repository root, the Makefile wires up a full local environment
(builds the image, deploys an in-cluster DMTF Redfish mockup pre-loaded with
`testdata/pdu-mockup`, creates the credentials Secret, and installs the chart):

```bash
make kind-up      # create cluster + build/load image + mockup + deploy chart
make kind-verify  # port-forward and probe the running pod
make kind-down    # delete the cluster
```

`helm test rootstock -n rootstock` runs an in-cluster smoke test against
`/healthz` and `/ready`.
