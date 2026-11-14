#!/usr/bin/env bash
#
# Prints the cloud-init user data for nodes launched by Karpenter on a
# CloudStack Kubernetes Service (CKS) cluster.
#
# It is the user data of an existing CKS worker, which already knows how to
# install Kubernetes from the binaries ISO and join the cluster, plus what a
# plain cloud image lacks compared to the CKS node image: containerd and the
# kubeadm prerequisites. See docs/CKS.md.
#
# The output contains the cluster bootstrap token. Treat it as a secret.
#
# Requires cmk (configured for the account that owns the cluster) and jq.

set -euo pipefail

worker_vm_id="${1:?usage: $0 <id of a worker VM of the CKS cluster>}"

userdata="$(cmk -o json get virtualmachineuserdata virtualmachineid="${worker_vm_id}" |
	jq -er '(.virtualmachineuserdata // .).userdata' | base64 -d)"

if [[ "$(grep -c '^runcmd:$' <<<"${userdata}")" != 1 ]]; then
	echo "unexpected user data: expected exactly one top-level 'runcmd:' key" >&2
	exit 1
fi
if ! grep -q 'kubeadm join' <<<"${userdata}"; then
	echo "unexpected user data: no 'kubeadm join' found, is ${worker_vm_id} a CKS worker?" >&2
	exit 1
fi

# Everything before runcmd
sed '/^runcmd:$/,$d' <<<"${userdata}"

cat <<'EOF'
package_update: true
packages:
  - curl
  - ca-certificates

runcmd:
  - |
    set -e
    . /etc/os-release
    # kubeadm preflight requires IP forwarding; the CKS scripts edit /etc/sysctl.conf
    sysctl -w net.ipv4.ip_forward=1
    echo "net.ipv4.ip_forward = 1" > /etc/sysctl.d/99-kubernetes.conf
    touch /etc/sysctl.conf
    # containerd 1.7, as on the CKS node image, and the kubeadm prerequisites
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL "https://download.docker.com/linux/${ID}/gpg" -o /etc/apt/keyrings/docker.asc
    echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable" > /etc/apt/sources.list.d/docker.list
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y "containerd.io=1.7.*" conntrack socat ethtool ebtables ipset iptables ipvsadm
    # Karpenter expects nodes to register with this taint and removes it once
    # it has synced the labels and taints of the NodeClaim
    grep -q 'KUBELET_EXTRA_ARGS=--cgroup-driver=systemd' /opt/bin/setup-kube-system
    sed -i 's|KUBELET_EXTRA_ARGS=--cgroup-driver=systemd|& --register-with-taints=karpenter.sh/unregistered:NoExecute|' /opt/bin/setup-kube-system
EOF

# The original runcmd entries
sed '1,/^runcmd:$/d' <<<"${userdata}"
