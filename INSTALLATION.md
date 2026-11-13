# Installation Guide

This guide will walk you through installing the Karpenter CloudStack Provider in your Kubernetes cluster.

These instructions deploy from the fork at
[ewerton-silva00/karpenter-provider-cloudstack](https://github.com/ewerton-silva00/karpenter-provider-cloudstack).
The upstream project never published a Helm repository, chart or container
image, so the chart is installed from a local clone and the image comes from
this fork's GitHub Container Registry.

> **Status: experimental.** The controller starts and connects to CloudStack
> (validated on a CloudStack Kubernetes Service cluster running Kubernetes
> 1.36). Node provisioning does **not** work yet: created VMs are not
> bootstrapped to join the cluster and are never matched to a `Node`. Do not
> create a `NodePool` on a cluster you care about — Karpenter would create and
> destroy VMs in a loop.

## Prerequisites

- Kubernetes cluster version 1.28 or later
- CloudStack 4.22 or later
- kubectl configured to access your cluster
- Helm 3.x installed
- git
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

### 1. Clone the Fork

There is no Helm repository; the chart is installed from the source tree.

```bash
git clone https://github.com/ewerton-silva00/karpenter-provider-cloudstack.git
cd karpenter-provider-cloudstack
```

### 2. Choose a Controller Image

A development image is published to GitHub Container Registry and is public:

```
ghcr.io/ewerton-silva00/cloudstack/karpenter/controller:dev-69b9cfa
```

The tag suffix is the commit the image was built from. To build and publish
your own image instead (requires a token with the `write:packages` scope):

```bash
gh auth token | docker login ghcr.io -u <your-user> --password-stdin

make docker-build REGISTRY=ghcr.io/<your-user> IMAGE_TAG=dev
docker push ghcr.io/<your-user>/cloudstack/karpenter/controller:dev
```

A newly pushed GHCR package is private. Make it public in the package
settings, or set `imagePullSecrets` in the chart values.

### 3. Create Namespace

```bash
kubectl create namespace karpenter
```

### 4. Install Karpenter CloudStack Provider

The chart's default `image.repository` still points to the upstream registry,
where no image exists, so the image must be set explicitly. Prefer a values
file (see [Configuration Options](#configuration-options)) over `--set` for
the credentials, to keep them out of your shell history.

```bash
helm install karpenter-cloudstack ./charts/karpenter-cloudstack \
  --namespace karpenter \
  --set image.repository=ghcr.io/ewerton-silva00/cloudstack/karpenter/controller \
  --set image.tag=dev-69b9cfa \
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

### 5. Verify Installation

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
# Controller Image
image:
  repository: ghcr.io/ewerton-silva00/cloudstack/karpenter/controller
  tag: dev-69b9cfa

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
helm install karpenter-cloudstack ./charts/karpenter-cloudstack \
  --namespace karpenter \
  --values values.yaml
```

## Creating Your First NodeClass

> Service offering selection by `tags` is not implemented yet; select service
> offerings by `name` or `id`. See the status note at the top before creating
> a `NodePool`.

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
helm upgrade karpenter-cloudstack ./charts/karpenter-cloudstack \
  --namespace karpenter \
  --set cloudstack.verifySSL=false \
  --reuse-values
```

## Upgrading

To upgrade to a new version, update your clone and point the release at the
new image tag:

```bash
git pull
helm upgrade karpenter-cloudstack ./charts/karpenter-cloudstack \
  --namespace karpenter \
  --reuse-values \
  --set image.tag=<new-tag>
```

Helm does not upgrade CRDs. When the files under
`charts/karpenter-cloudstack/crds/` change, apply them manually:

```bash
kubectl apply --server-side -f charts/karpenter-cloudstack/crds/
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

