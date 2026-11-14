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

package instance

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

func instanceType(name string, cpu, memory string, price float64, available bool) *cloudprovider.InstanceType {
	return &cloudprovider.InstanceType{
		Name: name,
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, name),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "zone-1"),
			scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
		),
		Offerings: cloudprovider.Offerings{{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "zone-1"),
				scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
			),
			Price:     price,
			Available: available,
		}},
		Capacity: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
			corev1.ResourcePods:   resource.MustParse("110"),
		},
		Overhead: &cloudprovider.InstanceTypeOverhead{},
	}
}

func nodeClaim(cpu, memory string, reqs ...karpv1.NodeSelectorRequirementWithMinValues) *karpv1.NodeClaim {
	return &karpv1.NodeClaim{Spec: karpv1.NodeClaimSpec{
		Requirements: reqs,
		Resources: karpv1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		}},
	}}
}

func requirement(key string, values ...string) karpv1.NodeSelectorRequirementWithMinValues {
	return karpv1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpIn, Values: values},
	}
}

func TestSelectInstanceType(t *testing.T) {
	small := instanceType("small", "2", "4Gi", 0.10, true)
	medium := instanceType("medium", "4", "8Gi", 0.20, true)
	large := instanceType("large", "8", "16Gi", 0.40, true)
	all := []*cloudprovider.InstanceType{large, small, medium}

	tests := []struct {
		name          string
		nodeClaim     *karpv1.NodeClaim
		instanceTypes []*cloudprovider.InstanceType
		want          string
	}{
		{"cheapest that fits", nodeClaim("1", "1Gi"), all, "small"},
		{"skips types too small for the requests", nodeClaim("3", "1Gi"), all, "medium"},
		{"memory is taken into account", nodeClaim("1", "12Gi"), all, "large"},
		{"honours the instance type requirement", nodeClaim("1", "1Gi", requirement(corev1.LabelInstanceTypeStable, "medium", "large")), all, "medium"},
		{"honours the zone requirement", nodeClaim("1", "1Gi", requirement(corev1.LabelTopologyZone, "zone-2")), all, ""},
		{"nothing fits", nodeClaim("16", "1Gi"), all, ""},
		{"no instance types", nodeClaim("1", "1Gi"), nil, ""},
		{"skips unavailable offerings", nodeClaim("1", "1Gi"), []*cloudprovider.InstanceType{instanceType("small", "2", "4Gi", 0.10, false), medium}, "medium"},
		{"ties are broken by name", nodeClaim("1", "1Gi"), []*cloudprovider.InstanceType{instanceType("b", "2", "4Gi", 0.10, true), instanceType("a", "2", "4Gi", 0.10, true)}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if it := selectInstanceType(tt.nodeClaim, tt.instanceTypes); it != nil {
				got = it.Name
			}
			if got != tt.want {
				t.Errorf("selectInstanceType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVMName(t *testing.T) {
	if got, ok := NodeClaimNameFromVMName(VMName("default-abc12")); !ok || got != "default-abc12" {
		t.Errorf("round trip = %q, %v", got, ok)
	}
	for _, name := range []string{"cks-apps-node-1a1d6d464d1", "karpenter-", "", "my-karpenter-node"} {
		if got, ok := NodeClaimNameFromVMName(name); ok {
			t.Errorf("NodeClaimNameFromVMName(%q) = %q, true; want false", name, got)
		}
	}
}
