# Installation Guide

This guide will walk you through installing the Karpenter CloudStack Provider in your Kubernetes cluster.

These instructions deploy from the fork at
[ewerton-silva00/karpenter-provider-cloudstack](https://github.com/ewerton-silva00/karpenter-provider-cloudstack).
The upstream project never published a Helm repository, chart or container
image, so both the chart and the image come from this fork's GitHub Container
Registry.

> **Status: experimental.** Node provisioning was validated end to end only on
> CloudStack Kubernetes Service (CKS), where it needs the setup described in
> [docs/CKS.md](./docs/CKS.md). On other kinds of cluster the user data must
> make the VM join the cluster and set its provider ID, which is untested.
> Always set `limits` on a `NodePool`.

## Prerequisites

- Kubernetes cluster version 1.28 or later
- CloudStack 4.22 or later
- kubectl configured to access your cluster
- Helm 3.8 or later (OCI registry support)
- CloudStack API credentials with appropriate permissions

## CloudStack Permissions

The CloudStack API credentials need the following permissions:

- Virtual Machine: Deploy, List, Destroy
- Service Offering: List
- Template: List
- Network: List
- Zone: List
- Tags: Create, List, Delete

## Installation Steps

### 1. Create Namespace

```bash
kubectl create namespace karpenter
```

### 2. Install Karpenter CloudStack Provider

The chart is published as an OCI artifact; there is no `helm repo add` step.
Its defaults point to the controller image published by this fork
(`ghcr.io/ewerton-silva00/cloudstack/karpenter/controller`, tagged with the
commit it was built from).

Prefer a values file (see [Configuration Options](#configuration-options))
over `--set` for the credentials, to keep them out of your shell history.

```bash
helm install karpenter-cloudstack \
  oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version 0.2.0 \
  --namespace karpenter \
  --set cloudstack.apiUrl=https://your-cloudstack-api.com/client/api \
  --set cloudstack.apiKey=YOUR_API_KEY \
  --set cloudstack.secretKey=YOUR_SECRET_KEY \
  --set clusterName=my-cluster
```

The chart installs the Karpenter core CRDs (`NodePool`, `NodeClaim`,
`NodeOverlay`) together with `CloudStackNodeClass`.

On a CloudStack Kubernetes Service (CKS) cluster, the API URL and keys the
cluster already uses are stored in the `cloudstack-secret` secret in
`kube-system` (key `cloud-config`).

### 3. Verify Installation

```bash
kubectl get pods --namespace karpenter
kubectl logs --namespace karpenter -l app.kubernetes.io/name=karpenter-cloudstack
```

You should see the controller `1/1 Running` and logs indicating it has
connected to CloudStack (`CloudStack operator initialized successfully`,
followed by `Listed instances`).

## Configuration Options

### values.yaml Configuration

Create a `values.yaml` file with your configuration:

```yaml
# CloudStack Configuration
cloudstack:
  apiUrl: "https://your-cloudstack-api.com/client/api"
  apiKey: "your-api-key"
  secretKey: "your-secret-key"
  verifySSL: true

# Cluster Configuration
clusterName: "my-cluster"

# Controller Resources
resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 256Mi

# Log Level
logLevel: info
```

Install with custom values:

```bash
helm install karpenter-cloudstack \
  oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version 0.2.0 \
  --namespace karpenter \
  --values values.yaml
```

### Installing from Source with Your Own Image

To run modified code, build and publish your own image (requires a token with
the `write:packages` scope) and install the chart from a clone:

```bash
git clone https://github.com/ewerton-silva00/karpenter-provider-cloudstack.git
cd karpenter-provider-cloudstack

gh auth token | docker login ghcr.io -u <your-user> --password-stdin
make docker-build REGISTRY=ghcr.io/<your-user> IMAGE_TAG=dev
docker push ghcr.io/<your-user>/cloudstack/karpenter/controller:dev

helm install karpenter-cloudstack ./charts/karpenter-cloudstack \
  --namespace karpenter \
  --set image.repository=ghcr.io/<your-user>/cloudstack/karpenter/controller \
  --set image.tag=dev \
  --values values.yaml
```

A newly pushed GHCR package is private. Make it public in the package
settings, or set `imagePullSecrets` in the chart values.

## Creating Your First NodeClass

> On CloudStack Kubernetes Service, follow [docs/CKS.md](./docs/CKS.md) instead
> of this section and the next one.
>
> Service offering selection by `tags` is not implemented yet; select service
> offerings by `name` or `id`.

After installation, create a CloudStackNodeClass:

```bash
kubectl apply -f - <<EOF
apiVersion: karpenter.k8s.cloudstack/v1
kind: CloudStackNodeClass
metadata:
  name: default
spec:
  zone: zone-01
  networkSelectorTerms:
    - tags:
        karpenter.sh/discovery: my-cluster
  serviceOfferingSelectorTerms:
    - tags:
        karpenter.sh/discovery: my-cluster
  templateSelectorTerms:
    - tags:
        os: ubuntu
        version: "22.04"
  userData: |
    #!/bin/bash
    echo "Karpenter node initialization"
EOF
```

## Creating Your First NodePool

Create a NodePool that references the NodeClass:

```bash
kubectl apply -f - <<EOF
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    spec:
      nodeClassRef:
        group: karpenter.k8s.cloudstack
        kind: CloudStackNodeClass
        name: default
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
        - key: node.kubernetes.io/instance-type
          operator: In
          values: ["Medium Instance", "Large Instance"]
  limits:
    cpu: "100"
    memory: 100Gi
EOF
```

## Testing

Deploy a test workload to verify Karpenter is working:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: inflate
spec:
  replicas: 5
  selector:
    matchLabels:
      app: inflate
  template:
    metadata:
      labels:
        app: inflate
    spec:
      containers:
      - name: inflate
        image: public.ecr.aws/eks-distro/kubernetes/pause:3.7
        resources:
          requests:
            cpu: 1
            memory: 2Gi
EOF
```

Watch for new nodes being created:

```bash
kubectl get nodes --watch
kubectl get nodeclaims --watch
```

## Tagging CloudStack Resources

For Karpenter to discover CloudStack resources, you need to tag them appropriately:

### Networks

Tag your networks with:
```
karpenter.sh/discovery: my-cluster
```

### Service Offerings

Tag your service offerings with:
```
karpenter.sh/discovery: my-cluster
```

### Templates

Tag your templates with appropriate metadata:
```
os: ubuntu
version: 22.04
karpenter.sh/discovery: my-cluster
```

## Troubleshooting

### Controller Not Starting

Check logs:
```bash
kubectl logs -n karpenter -l app.kubernetes.io/name=karpenter-cloudstack
```

Common issues:
- Incorrect CloudStack API credentials
- Network connectivity to CloudStack API
- SSL certificate verification issues

### Nodes Not Being Created

Check NodeClass status:
```bash
kubectl get cloudstacknodeclass default -o yaml
```

Check NodeClaim events:
```bash
kubectl get events --sort-by='.lastTimestamp' | grep NodeClaim
```

### SSL Certificate Issues

If you have self-signed certificates:
```bash
helm upgrade karpenter-cloudstack \
  oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version 0.2.0 \
  --namespace karpenter \
  --set cloudstack.verifySSL=false \
  --reuse-values
```

## Upgrading

To upgrade to a new chart version:

```bash
helm upgrade karpenter-cloudstack \
  oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version <new-version> \
  --namespace karpenter \
  --reuse-values
```

Helm does not upgrade CRDs. When a new chart version changes them, apply them
manually:

```bash
helm pull oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version <new-version> --untar
kubectl apply --server-side -f karpenter-cloudstack/crds/
```

## Uninstallation

To remove Karpenter CloudStack Provider:

```bash
# Delete all NodePools and NodeClasses first
kubectl delete nodepools --all
kubectl delete cloudstacknodeclasses --all

# Uninstall Helm chart
helm uninstall karpenter-cloudstack --namespace karpenter

# Delete namespace
kubectl delete namespace karpenter
```

## Next Steps

- Review the [examples](./examples/) directory for advanced configurations
- Read the [CloudStackNodeClass specification](./docs/nodeclass-spec.md)
- Check out [best practices](./docs/best-practices.md)
- Join the community discussions

