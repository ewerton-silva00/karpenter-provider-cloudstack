# Karpenter on CloudStack Kubernetes Service

This guide explains how to make Karpenter launch nodes for a cluster created by
CloudStack Kubernetes Service (CKS). It was validated on CloudStack 4.22 with a
CKS cluster running Kubernetes 1.36 and Cilium.

Install the controller first; see [INSTALLATION.md](../INSTALLATION.md).

## How it works

CKS builds its nodes from the system VM template. CloudStack does not allow
that template to be deployed as a regular VM, and registering the same image as
a user template does not help: outside CKS it boots as an unconfigured system
VM and ignores the user data. Karpenter therefore launches nodes from a regular
cloud image and reproduces what CKS does to turn a VM into a node:

1. The VM is deployed with the **user data of a CKS worker**, which installs
   Kubernetes and runs `kubeadm join`, extended with what a plain cloud image
   lacks: containerd and the kubeadm prerequisites.
2. The **Kubernetes binaries ISO** of the cluster version is attached. The CKS
   scripts install kubeadm, kubelet and the container images from it.
3. The node joins. CKS kubelets do not use an external cloud provider, so
   Karpenter itself sets `spec.providerID` on the node, which is what links it
   to its `NodeClaim`, and then detaches the ISO.

A node takes about three minutes to become ready.

## Limitations

- **CKS does not know these VMs.** They are not counted in the cluster size, not
  upgraded with the cluster and not removed when the cluster is deleted. Delete
  the `NodePool`s before deleting the cluster.
- **The user data embeds the cluster bootstrap token**, which CKS creates
  without an expiry date. Anyone who can read the `CloudStackNodeClass` can join
  a node to the cluster. Restrict access to that resource.
- **Booting needs internet access**, to install containerd from
  `download.docker.com`. A template with containerd already installed removes
  that dependency and shortens the boot.
- Karpenter nodes run a different OS image than the CKS nodes.

## Prerequisites

- `cmk` configured for the account that owns the cluster, and `jq`.
- A cloud-init template whose kernel can read the ISO. **Ubuntu 24.04** works.
  Debian `genericcloud` does not: its kernel has no CD-ROM driver, so the node
  waits for the ISO forever.
- The values below, all visible with `cmk`:

  | Value | How to find it |
  |---|---|
  | Zone and network of the cluster | `cmk list kubernetesclusters name=<cluster> filter=zonename,networkid` |
  | ID of a worker VM | `cmk list virtualmachines keyword=<cluster>-node filter=id,name` |
  | ISO of the cluster version | `cmk list kubernetessupportedversions filter=name,isoname` |
  | Service offerings | `cmk list serviceofferings filter=name,cpunumber,memory` |

## 1. Create the CloudStackNodeClass

Copy [examples/cks/nodeclass.yaml](../examples/cks/nodeclass.yaml), set the zone,
network, service offerings, template and ISO, and apply it:

```bash
kubectl apply -f nodeclass.yaml
```

Generate the user data from a worker of the cluster and add it to the node
class. It contains the bootstrap token, so do not commit it:

```bash
hack/cks-userdata.sh <worker-vm-id> > userdata.yaml

kubectl patch cloudstacknodeclass cks --type=merge \
  -p "$(jq -n --rawfile ud userdata.yaml '{spec: {userData: $ud}}')"

rm userdata.yaml
```

Check that the zone, network and template were resolved:

```bash
kubectl get cloudstacknodeclass cks -o jsonpath='{.status.conditions}'
```

## 2. Create the NodePool

Apply [examples/cks/nodepool.yaml](../examples/cks/nodepool.yaml) after adjusting
the limits:

```bash
kubectl apply -f examples/cks/nodepool.yaml
```

Two settings matter on CKS:

- **`startupTaints` for Cilium.** The Cilium operator taints every new node with
  `node.cilium.io/agent-not-ready:NoSchedule` until the agent is running.
  Without the startup taint, Karpenter concludes that the pending pods do not
  fit the node it just launched and launches another one for the same pods.
- **`limits`.** They bound how much a misconfiguration can launch.

## 3. Verify

Create pods that do not fit the existing nodes and watch a node arrive:

```bash
kubectl get nodeclaims --watch
kubectl get nodes --watch
```

A `NodeClaim` should go through `Launched`, `Registered` and `Initialized`, and
exactly one should be created for a set of pods that fits one node.

## Troubleshooting

**The VM runs but the node never appears.** Log in to the VM (user `cloud`,
with the SSH key of the cluster) and read `/var/log/cloud-init-output.log`.
Repeated `Waiting for Binaries directory /mnt/k8sdisk/` means the ISO is not
visible: check that it is attached, and that `lsblk` shows a CD-ROM device. If
it does not, the template kernel cannot read it.

**Two nodes are launched for pods that fit one.** A taint that Karpenter does
not know about was on the node when it registered. Compare the taints of a new
node with the `startupTaints` of the `NodePool`.

**`missing taint prevents registration-related race conditions`** in the
controller log. The kubelet did not register with the
`karpenter.sh/unregistered` taint. `hack/cks-userdata.sh` adds it; the message
appears when the user data was written by hand or the CKS script changed.

**The NodeClaim is deleted after 15 minutes.** The node did not register in
time. See the first item.
