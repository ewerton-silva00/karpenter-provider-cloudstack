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

Known broken — node provisioning does not work end to end:

- Nodes are matched to `NodeClaim`s by `spec.providerID`. CKS nodes have no
  `providerID`, and the format built by `FormatProviderID`
  (`cloudstack://<zone>/<id>`) is unverified against the CloudStack CCM.
- No bootstrap: `userData` is passed through as is. Nothing makes the VM join
  the cluster (`kubeadm join`), and CKS does not know about the VM.
- `selectInstanceType` (`pkg/providers/instance/instance.go`) returns the first
  instance type and ignores the `NodeClaim` requirements.
- Service offering selection by tags is not implemented
  (`pkg/providers/instancetype/instancetype.go`).
- `Create` tags the VM only after it is `Running`; a failure in between leaves
  an untagged VM that `List` and garbage collection cannot see.
- `instanceToNodeClaim` does not set capacity/allocatable; prices are made up.

Do not create a `NodePool` on a shared cluster until the first two are fixed:
Karpenter would create and destroy VMs in a loop.

## Commands

```bash
go build ./...            # compile
go vet ./...
make test                 # go test -v ./... (there are no tests yet)
make lint                 # golangci-lint; `make tools` installs it
make generate             # deepcopy + CloudStackNodeClass CRD into the chart
helm lint charts/karpenter-cloudstack
helm template karpenter charts/karpenter-cloudstack -n karpenter --include-crds
```

Image (the Makefile defaults to the upstream registry, override it):

```bash
make docker-build REGISTRY=ghcr.io/ewerton-silva00 IMAGE_TAG=dev-<short-sha>
docker push ghcr.io/ewerton-silva00/cloudstack/karpenter/controller:dev-<short-sha>
```

Deployment from the fork is described in `INSTALLATION.md`. Keep the image tag
there in sync when publishing a new image.

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
