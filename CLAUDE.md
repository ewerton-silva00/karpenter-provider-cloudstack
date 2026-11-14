# CLAUDE.md

Karpenter provider for Apache CloudStack. This repository is a fork of
`mperea/karpenter-provider-cloudstack` (`upstream` remote); work lands on the
fork (`origin`, `ewerton-silva00/karpenter-provider-cloudstack`).

## Status

Experimental. Upstream was never run on a real cluster: it had no releases, no
published image or chart, and no tests. Treat existing code and docs as
unverified until exercised.

Working (validated on a CloudStack Kubernetes Service cluster, Kubernetes 1.36):

- Controller starts, wins leader election, connects to CloudStack and lists VMs.
- Chart installs CRDs, RBAC and the deployment.

Node provisioning is validated end to end on CKS only (scale up, instance type
selection, NodePool limits, consolidation, scale to zero). `docs/CKS.md` is the
user-facing guide; the reasons behind the design:

- CKS nodes cannot be reproduced from the CKS image. CloudStack refuses to
  deploy the system VM template as a user VM, and registered as a user template
  it boots unconfigured and ignores the user data. Nodes are launched from a
  cloud image (Ubuntu 24.04 works; Debian `genericcloud` has no CD-ROM driver,
  so the ISO is invisible) with the user data of a CKS worker plus containerd
  (`hack/cks-userdata.sh`), and the Kubernetes binaries ISO attached
  (`spec.iso`).
- CKS kubelets run without an external cloud provider, so nothing sets
  `spec.providerID`. `pkg/controllers/node/providerid` sets it from the
  NodeClaim, found through the node name (`karpenter-<nodeclaim>`), then
  detaches the ISO.
- The Cilium operator taints new nodes with
  `node.cilium.io/agent-not-ready:NoSchedule`. Unless the NodePool lists it in
  `startupTaints`, Karpenter launches a duplicate node for the same pods.

Known gaps:

- Service offering selection by tags is not implemented
  (`pkg/providers/instancetype/instancetype.go`).
- Prices are made up, and instance type memory is the service offering memory,
  slightly more than what the node reports.
- VMs launched by Karpenter are unknown to CKS: they are not counted, upgraded
  or removed with the cluster.
- The CKS `userData` embeds a bootstrap token that never expires.

Do not create a `NodePool` without `limits` on a shared cluster.

## Commands

```bash
go build ./...            # compile
go vet ./...
make test                 # go test -v ./...
make lint                 # golangci-lint; `make tools` installs it
make generate             # deepcopy + CloudStackNodeClass CRD into the chart (controller-gen v0.19.0)
helm lint charts/karpenter-cloudstack
helm template karpenter charts/karpenter-cloudstack -n karpenter --include-crds
```

Image (the Makefile defaults to the upstream registry, override it):

```bash
make docker-build REGISTRY=ghcr.io/ewerton-silva00 IMAGE_TAG=dev-<short-sha>
docker push ghcr.io/ewerton-silva00/cloudstack/karpenter/controller:dev-<short-sha>
```

Chart (published as an OCI artifact; the chart's `appVersion` is the default
image tag, so it must name an image that exists):

```bash
helm package charts/karpenter-cloudstack -d <out-dir>
helm push <out-dir>/karpenter-cloudstack-<version>.tgz oci://ghcr.io/ewerton-silva00/charts
```

Bump `version` in `Chart.yaml` for every published change; do not overwrite a
published version. Deployment from the fork is described in `INSTALLATION.md`;
keep the chart version there in sync.

## Layout

- `cmd/controller/main.go` — wires the core Karpenter controllers and the
  provider's own controllers.
- `pkg/operator` — builds the CloudStack client and providers; configuration
  comes from environment variables (`pkg/operator/options`).
- `pkg/cloudprovider` — implements the `sigs.k8s.io/karpenter` `CloudProvider`
  interface.
- `pkg/providers/{instance,instancetype,network,template,zone}` — CloudStack
  lookups and VM lifecycle, each with an in-memory cache.
- `pkg/controllers/nodeclass` — reconciles `CloudStackNodeClass` status.
- `pkg/controllers/node/providerid` — sets `spec.providerID` on nodes launched
  by Karpenter.
- `pkg/apis/v1` — `CloudStackNodeClass` types.
- `charts/karpenter-cloudstack` — Helm chart; `crds/` holds the provider CRD
  and the Karpenter core CRDs.

## Things that are easy to get wrong

- **Scheme**: Karpenter core uses the global client-go scheme. API types must
  be registered there (`pkg/apis/v1/doc.go`), or the controller panics at
  startup.
- **Ports**: Karpenter core serves metrics on 8080 and health probes on 8081.
- **RBAC**: `templates/rbac.yaml` mirrors the upstream core chart
  (`kwok/charts/templates` in the `sigs.k8s.io/karpenter` module). Re-check it
  when bumping the Karpenter version.
- **Core CRDs**: `karpenter.sh_*.yaml` under the chart's `crds/` are copied
  from `pkg/apis/crds` of the `sigs.k8s.io/karpenter` version pinned in
  `go.mod`. Copy them again on a version bump; `make generate` only produces
  the provider CRD.
- **CloudStack client**: providers type-assert `csClient.(*csapi.Client)`
  to reach the SDK's `New...Params` helpers, so the `CloudStackAPI` interface
  cannot be swapped for `pkg/fake` without a panic.
- **`main.go` passes `nil` as the `InstanceTypeStore`**: fine while the
  `NodeOverlay` feature gate is off.
- **Dead chart values**: `webhook` and `service` in `values.yaml` are not used
  by any template.

## Conventions

- Code, comments, commit messages and docs are in English.
- Conventional Commits (`fix(chart): ...`, `feat: ...`), as in the existing
  history.
- Commits are signed. If signing fails, stop and report it; do not bypass it.
- Never commit CloudStack credentials or rendered values files.
- Verify changes against a real cluster where possible: `helm lint` and
  `go build` passing did not catch any of the startup bugs found so far.
