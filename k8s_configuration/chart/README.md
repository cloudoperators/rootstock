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
make kind-up      # create cluster + build/load image + mockup + deploy chart
make kind-verify  # port-forward and probe the running pod
make kind-down    # delete the cluster
```

`helm test rootstock -n rootstock` runs an in-cluster smoke test against
`/healthz` and `/ready`.

**alternatively**

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

## Secrets: the Redfish password

rootstock reads the Redfish password **only** from a mounted file
(`--config-folder <dir>` + `--redfish-credentials-file <name>`, i.e.
`<configFolder>/<credentialsFileName>`, default `/config/redfish_password`). The
file is watched (fsnotify) and hot-reloaded on change. The chart therefore
always projects the password as a **read-only mounted file** and never sets it
as an environment variable.

There are two mutually exclusive ways to provide the password:

| Mode                | Values                           | Use case                                                                                                                                           |
| ------------------- | -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Existing Secret** | `redfish.existingSecret: <name>` | Production, CI with a pre-created Secret. Recommended when the Secret is managed outside the chart.                                                |
| **Inline**          | `redfish.password: <value>`      | Self-contained / local (kind) runs **and Greenhouse** (`valueFrom.secret` feeds the `redfish.password` value). The chart renders a Secret for you. |

In both modes the Secret is mounted at `redfish.configFolder` and the key
`redfish.secretKey` is projected to the file `redfish.credentialsFileName` with
mode `0400`.

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
Because the password arrives as the `redfish.password` value (not
`redfish.existingSecret`), this uses the **Inline** mode above: the chart
renders the Secret from that value. Leave `redfish.existingSecret` unset.

## Values

See [`values.yaml`](./values.yaml) for the full list and inline documentation.
Key options:

| Key                             | Default                            | Description                                  |
| ------------------------------- | ---------------------------------- | -------------------------------------------- |
| `image.repository`              | `ghcr.io/cloudoperators/rootstock` | Image repository.                            |
| `image.tag`                     | `""` (chart appVersion)            | Image tag.                                   |
| `redfish.username`              | `redfish`                          | Redfish auth username.                       |
| `redfish.existingSecret`        | `""`                               | Pre-created Secret holding the password.     |
| `redfish.password`              | `""`                               | Inline password (renders a Secret).          |
| `redfish.secretKey`             | `redfish_password`                 | Key within the Secret.                       |
| `redfish.configFolder`          | `/config`                          | Mount dir (`--config-folder`).               |
| `redfish.credentialsFileName`   | `redfish_password`                 | File name (`--redfish-credentials-file`).    |
| `server.port`                   | `9220`                             | REST server port.                            |
| `server.scrapeTimeout`          | `30s`                              | Upstream scrape timeout.                     |
| `service.type` / `service.port` | `ClusterIP` / `9220`               | Service exposure.                            |
| `serviceMonitor.enabled`        | `false`                            | Create a Prometheus Operator ServiceMonitor. |
| `snmpExporter.enabled`          | `false`                            | Deploy SNMP exporter as sidecar.             |

## SNMP Exporter (Optional Sidecar)

The chart includes an optional SNMP exporter sidecar container for monitoring
network devices via SNMP alongside Redfish monitoring. This is disabled by default.

### Architecture

When enabled, the SNMP exporter runs as a sidecar container in the same pod as
rootstock. Both containers share the pod and are accessible through a single Service:

- **rootstock**: Port 9220 (Redfish `/probe` endpoint)
- **snmp-exporter**: Port 9116 (SNMP `/snmp` endpoint)

### Enabling SNMP Exporter

```bash
helm install rootstock k8s_configuration/chart \
  --set snmpExporter.enabled=true \
  --set redfish.password='s3cr3t'
```

### Configuration

Customize the SNMP exporter sidecar configuration:

**Option 1: Inline configuration (simple)**

```yaml
snmpExporter:
  enabled: true
  service:
    port: 9116
  config:
    # Your custom snmp.yml configuration (must have 'modules' wrapper)
    modules:
      if_mib:
        walk:
          - 1.3.6.1.2.1.2
          - 1.3.6.1.2.1.31.1.1
        metrics:
          - name: ifNumber
            oid: 1.3.6.1.2.1.2.1
            type: gauge
            help: Number of network interfaces
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
```

**Option 2: External file (recommended for complex configs)**

1. Create `files/snmp.yml` in the chart directory
2. Configure to use the file:

```yaml
snmpExporter:
  enabled: true
  configFile: "files/snmp.yml"  # Use external file instead of inline config
```

See `files/snmp.yml` and `files/README.md` for examples and generator instructions.

### Prometheus Scrape Configuration

Use the multi-target pattern to scrape SNMP devices. Since both containers run
in the same pod, use the same service name with different ports:

```yaml
scrape_configs:
  # Rootstock Redfish monitoring
  - job_name: 'rootstock'
    static_configs:
      - targets: ['http://pdu.example.com']
    metrics_path: /probe
    relabel_configs:
      - target_label: __address__
        replacement: rootstock:9220
  
  # SNMP device monitoring (sidecar)
  - job_name: 'snmp'
    static_configs:
      - targets: ['192.168.1.1']  # SNMP device
    metrics_path: /snmp
    params:
      module: [if_mib]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: rootstock:9116  # Same service, different port
```

## Endpoints

rootstock serves `/healthz`, `/ready`, `/metrics`, and the multi-target
`/probe?target=<redfish-url>&labels=<pdu_id>` on `server.port` (default: 9220).

When SNMP exporter sidecar is enabled, it exposes `/snmp?target=<snmp-device>&module=<snmp-module>`
for SNMP device probing on port `9116` (configurable via `snmpExporter.service.port`).
