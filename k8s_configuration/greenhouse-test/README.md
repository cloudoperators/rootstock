<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
SPDX-License-Identifier: Apache-2.0
-->

# Testing the rootstock image with Greenhouse (apeirolab node 192.168.0.98)

This runbook assumes you have (re)installed Greenhouse on the cluster. The
rootstock **image is already present** in the node's containerd:

```
docker.io/library/rootstock:0.1.0   (linux/arm64, imported via ctr -n k8s.io images import)
```

Because the image is NOT in a registry, every path below uses
`image.pullPolicy: Never` so the kubelet uses the local image.

## Key constraints discovered on this cluster

- Single-node kubeadm cluster, containerd runtime (no Docker), flannel CNI.
- No OCI registry on the node (only kube-apiserver :6443).
- Node architecture is **arm64** — the image is arm64-only; pods must schedule
  here.
- The node is **ephemeral**: a reprovision wipes containerd, so the image import
  (and anything else) must be repeated after a wipe. Never restart the node.

## Greenhouse requirements (must exist before the Plugin test)

1. Greenhouse operator + CRDs (`greenhouse.sap/*`, PluginDefinition, Plugin).
2. Flux (source-controller + helm-controller) — Greenhouse deploys Plugin charts
   via Flux.
3. The chart must be reachable by Flux. On this registry-less cluster that means
   either publishing the chart to an in-cluster-reachable OCI registry, or using
   the local-chart hostPath mechanism from Greenhouse's local-dev docs.

## Step 1 — Publish the chart so Flux can pull it

Flux pulls the chart from inside the cluster, so the chart must live in an OCI
registry the cluster can reach. If you stand up a registry on the node:

```bash
# package the chart locally
helm package k8s_configuration/chart          # -> rootstock-0.1.0.tgz
# push to the registry (adjust host/port; use TLS or an insecure-registry allowance)
helm push rootstock-0.1.0.tgz oci://<registry-host>:<port>/charts
```

Then set in k8s_configuration/plugindefinition.yaml:
`helmChart.repository: oci://<registry-host>:<port>/charts`, `name: rootstock`,
`version: 0.1.0`.

## Step 2 — Apply the PluginDefinition

```bash
export KUBECONFIG=<admin kubeconfig>   # on the node: /etc/kubernetes/admin.conf
# PluginDefinition is namespaced: it MUST live in the Organization namespace
# (the org whose name == the namespace), or it never reconciles.
kubectl -n <org-namespace> apply -f k8s_configuration/plugindefinition.yaml
kubectl -n <org-namespace> get plugindefinition rootstock
```

## Step 3 — Create the Redfish Secret (Organization namespace)

```bash
kubectl create secret generic rootstock-redfish \
  --from-literal=redfish_password='<password>' -n <org-namespace>
# or edit + apply k8s_configuration/greenhouse-test/redfish-secret.yaml
```

## Step 4 — Create the Plugin

Edit `k8s_configuration/greenhouse-test/plugin.yaml` and replace
`PLACEHOLDER_ORG_NAMESPACE` and `PLACEHOLDER_CLUSTER_NAME`, then:

```bash
kubectl apply -f k8s_configuration/greenhouse-test/plugin.yaml
```

Note the `image.pullPolicy: Never` optionValue — this is what makes the local
containerd image usable without a registry.

## Step 5 — Verify

```bash
kubectl get plugin rootstock -n <org-namespace> -o jsonpath='{.status}' ; echo
kubectl -n rootstock get pods
kubectl -n rootstock get pod -l app.kubernetes.io/name=rootstock \
  -o jsonpath='{.items[0].spec.securityContext}' ; echo   # expect UID/GID 65531
# functional probe:
kubectl -n rootstock port-forward svc/rootstock 9220:9220 &
curl -s http://localhost:9220/healthz
curl -s "http://localhost:9220/probe?target=<REDFISH_URL>&labels=1" | head
```

A reachable Redfish target is required for /probe. If none exists on this
cluster, deploy the mockup (hack/kind/redfish-mockup.yaml + its image, imported
the same way as the rootstock image) or point at a real PDU the pod can route
to.

## Fallback: prove the image without Greenhouse (plain helm)

This validates the exact image + chart on this node without needing
Flux/registry:

```bash
export KUBECONFIG=<admin kubeconfig>
helm install rootstock k8s_configuration/chart -n rootstock --create-namespace \
  --set image.repository=docker.io/library/rootstock \
  --set image.tag=0.1.0 \
  --set image.pullPolicy=Never \
  --set redfish.password='<password>'
```
