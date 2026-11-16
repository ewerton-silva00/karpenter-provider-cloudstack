/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudprovider

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"

	v1 "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/apis/v1"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/instance"
)

func TestProviderID(t *testing.T) {
	const id = "f2e4070f-b70c-4a83-93a5-0685c0c8a6cc"

	if got, err := ParseProviderID(FormatProviderID("RN-1a", id)); err != nil || got != id {
		t.Errorf("round trip = %q, %v; want %q", got, err, id)
	}

	for _, providerID := range []string{
		"",
		id,
		"cloudstack://",
		"cloudstack://" + id,
		"cloudstack://RN-1a/",
		"cloudstack:///" + id,
		"cloudstack://RN-1a/" + id + "/extra",
		"aws:///us-east-1a/i-0123456789",
	} {
		if got, err := ParseProviderID(providerID); err == nil {
			t.Errorf("ParseProviderID(%q) = %q, nil; want an error", providerID, got)
		}
	}
}

func TestInstanceToNodeClaim(t *testing.T) {
	inst := &instance.Instance{
		ID:              "vm-1",
		Zone:            "RN-1a",
		ServiceOffering: "KVM-2-4",
		Template:        "Ubuntu 24.04 LTS (Noble Numbat)",
		TemplateID:      "template-1",
		Tags:            map[string]string{v1.NodePoolTagKey: "default"},
	}
	instanceTypes := []*corecloudprovider.InstanceType{
		{
			Name:     "KVM-4-8",
			Capacity: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4")},
			Overhead: &corecloudprovider.InstanceTypeOverhead{},
		},
		{
			Name:     "KVM-2-4",
			Capacity: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			Overhead: &corecloudprovider.InstanceTypeOverhead{
				KubeReserved: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			},
		},
	}

	nc := (&CloudProvider{}).instanceToNodeClaim(inst, instanceTypes)

	if nc.Status.ProviderID != "cloudstack://RN-1a/vm-1" {
		t.Errorf("providerID = %q", nc.Status.ProviderID)
	}
	if got := nc.Labels[corev1.LabelInstanceTypeStable]; got != "KVM-2-4" {
		t.Errorf("instance type label = %q", got)
	}
	if got := nc.Labels["karpenter.sh/nodepool"]; got != "default" {
		t.Errorf("nodepool label = %q", got)
	}
	if got, ok := nc.Labels[v1.LabelTemplateName]; ok {
		t.Errorf("template name %q is not a valid label value and must be dropped", got)
	}
	if got := nc.Labels[v1.LabelTemplateID]; got != "template-1" {
		t.Errorf("template id label = %q", got)
	}
	if got := nc.Status.Capacity.Cpu().String(); got != "2" {
		t.Errorf("capacity cpu = %q, want 2", got)
	}
	if got := nc.Status.Allocatable.Cpu().String(); got != "1900m" {
		t.Errorf("allocatable cpu = %q, want 1900m", got)
	}

	// Without a matching instance type the capacity is left unset
	if nc := (&CloudProvider{}).instanceToNodeClaim(inst, nil); nc.Status.Capacity != nil {
		t.Errorf("capacity = %v, want nil", nc.Status.Capacity)
	}
}
