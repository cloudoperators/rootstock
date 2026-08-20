# Getting Started

Deploy rootstock to a Kubernetes cluster with Prometheus Operator installed.

## Prerequisites

| Requirement | Version |
|---|---|
| Go | 1.21+ |
| kubectl | 1.26+ |
| Helm | 3.12+ |
| Prometheus Operator | 0.65+ |

## Deploy with Helm

Add the chart repository and install:

```bash
helm repo add cobaltcore https://cloudoperators.github.io/charts
helm repo update

helm install rootstock cobaltcore/rootstock \
  --namespace rootstock \
  --create-namespace \
  --set config.targets.whitelist="pdu1.example.com,pdu2.example.com"
```

### Configure credentials

Rootstock reads PDU credentials from a Kubernetes Secret. Create it before installing:

```bash
kubectl create secret generic rootstock-pdu-credentials \
  --namespace rootstock \
  --from-literal=username=admin \
  --from-literal=password=<your-password>
```

Reference the secret in your Helm values:

```yaml
# values.yaml
credentials:
  secretName: rootstock-pdu-credentials
  usernameKey: username
  passwordKey: password
```

### Verify the deployment

```bash
kubectl rollout status deployment/rootstock -n rootstock
kubectl get pods -n rootstock
```

Check the health endpoint:

```bash
kubectl port-forward svc/rootstock 8080:8080 -n rootstock
curl http://localhost:8080/health
```

Expected response: `{"status":"ok"}`

## Configure Prometheus scraping

Rootstock uses the [multi-target pattern](https://prometheus.io/docs/guides/multi-target-exporter/). Each scrape request includes a `target` query parameter specifying the PDU host.

### ServiceMonitor

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: rootstock
  namespace: rootstock
spec:
  selector:
    matchLabels:
      app: rootstock
  endpoints:
    - port: http
      path: /metrics
      relabelings:
        - sourceLabels: [__address__]
          targetLabel: __param_target
        - sourceLabels: [__param_target]
          targetLabel: instance
        - targetLabel: __address__
          replacement: rootstock.rootstock.svc.cluster.local:8080
```

### Prometheus scrape config (without Operator)

```yaml
scrape_configs:
  - job_name: rootstock
    static_configs:
      - targets:
          - pdu1.example.com
          - pdu2.example.com
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: rootstock.rootstock.svc.cluster.local:8080
```

## Next steps

- [Architecture](./concepts/architecture) — understand the three-component pipeline
- [Design](./concepts/design) — goals, security model, and requirements
- [API Reference](./reference/api) — HTTP endpoints exposed by rootstock
