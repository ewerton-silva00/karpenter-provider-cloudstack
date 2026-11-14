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

package instancetype

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	v1 "github.com/mperea/karpenter-provider-cloudstack/pkg/apis/v1"
	csapi "github.com/mperea/karpenter-provider-cloudstack/pkg/cloudstack"
)

// Provider provides instance type information
type Provider interface {
	List(ctx context.Context, nodeClass *v1.CloudStackNodeClass) ([]*cloudprovider.InstanceType, error)
	Get(ctx context.Context, nodeClass *v1.CloudStackNodeClass, name string) (*cloudprovider.InstanceType, error)
}

// DefaultProvider implements the InstanceType Provider
type DefaultProvider struct {
	csClient csapi.CloudStackAPI
	cache    *cache.Cache
	mu       sync.RWMutex
}

// NewDefaultProvider creates a new instance type provider
func NewDefaultProvider(csClient csapi.CloudStackAPI, cache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		csClient: csClient,
		cache:    cache,
	}
}

// List returns all instance types (service offerings)
func (p *DefaultProvider) List(ctx context.Context, nodeClass *v1.CloudStackNodeClass) ([]*cloudprovider.InstanceType, error) {
	// Resolve service offerings from node class
	serviceOfferings, err := p.resolveServiceOfferings(ctx, nodeClass)
	if err != nil {
		return nil, err
	}

	// Convert to Karpenter instance types
	instanceTypes := make([]*cloudprovider.InstanceType, 0, len(serviceOfferings))
	for _, offering := range serviceOfferings {
		instanceType := p.convertToInstanceType(offering, nodeClass.Spec.Zone)
		instanceTypes = append(instanceTypes, instanceType)
	}

	log.FromContext(ctx).Info("Listed instance types", "count", len(instanceTypes))

	return instanceTypes, nil
}

// Get returns a specific instance type by name
func (p *DefaultProvider) Get(ctx context.Context, nodeClass *v1.CloudStackNodeClass, name string) (*cloudprovider.InstanceType, error) {
	instanceTypes, err := p.List(ctx, nodeClass)
	if err != nil {
		return nil, err
	}

	instanceType, found := lo.Find(instanceTypes, func(it *cloudprovider.InstanceType) bool {
		return it.Name == name
	})

	if !found {
		return nil, fmt.Errorf("instance type %s not found", name)
	}

	return instanceType, nil
}

// resolveServiceOfferings resolves service offerings based on node class selectors
func (p *DefaultProvider) resolveServiceOfferings(ctx context.Context, nodeClass *v1.CloudStackNodeClass) ([]*cloudstack.ServiceOffering, error) {
	// Check cache
	cacheKey := fmt.Sprintf("service-offerings-%s", nodeClass.Spec.Zone)
	if cached, found := p.cache.Get(cacheKey); found {
		allOfferings := cached.([]*cloudstack.ServiceOffering)
		return p.filterServiceOfferings(allOfferings, nodeClass.Spec.ServiceOfferingSelectorTerms), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock
	if cached, found := p.cache.Get(cacheKey); found {
		allOfferings := cached.([]*cloudstack.ServiceOffering)
		return p.filterServiceOfferings(allOfferings, nodeClass.Spec.ServiceOfferingSelectorTerms), nil
	}

	// Fetch service offerings from CloudStack
	params := p.csClient.(*csapi.Client).ServiceOffering.NewListServiceOfferingsParams()

	resp, err := p.csClient.ListServiceOfferings(params)
	if err != nil {
		return nil, fmt.Errorf("listing service offerings: %w", err)
	}

	// Cache all offerings
	p.cache.Set(cacheKey, resp.ServiceOfferings, cache.DefaultExpiration)

	// Filter based on selectors
	filtered := p.filterServiceOfferings(resp.ServiceOfferings, nodeClass.Spec.ServiceOfferingSelectorTerms)

	log.FromContext(ctx).Info("Resolved service offerings", "total", len(resp.ServiceOfferings), "matched", len(filtered))

	return filtered, nil
}

// filterServiceOfferings filters service offerings based on selector terms
func (p *DefaultProvider) filterServiceOfferings(offerings []*cloudstack.ServiceOffering, terms []v1.ServiceOfferingSelectorTerm) []*cloudstack.ServiceOffering {
	// Custom offerings have no fixed CPU or memory, so they cannot be described as an instance type
	offerings = lo.Reject(offerings, func(o *cloudstack.ServiceOffering, _ int) bool {
		return o.Iscustomized
	})

	var matched []*cloudstack.ServiceOffering

	for _, term := range terms {
		// Match by ID
		if term.ID != "" {
			offering, found := lo.Find(offerings, func(o *cloudstack.ServiceOffering) bool {
				return o.Id == term.ID
			})
			if found {
				matched = append(matched, offering)
				continue
			}
		}

		// Match by Name
		if term.Name != "" {
			offering, found := lo.Find(offerings, func(o *cloudstack.ServiceOffering) bool {
				return o.Name == term.Name
			})
			if found {
				matched = append(matched, offering)
				continue
			}
		}

		// Match by Tags - would need to fetch tags separately
		// For simplicity, if only tags are specified, we'll skip this for now
		// In production, you'd want to fetch tags for each offering
	}

	// Remove duplicates
	matched = lo.UniqBy(matched, func(o *cloudstack.ServiceOffering) string {
		return o.Id
	})

	return matched
}

// convertToInstanceType converts a CloudStack service offering to a Karpenter instance type
func (p *DefaultProvider) convertToInstanceType(offering *cloudstack.ServiceOffering, zone string) *cloudprovider.InstanceType {
	// Calculate capacity
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(int64(offering.Cpunumber), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(int64(offering.Memory)*1024*1024, resource.BinarySI), // MB to bytes
		corev1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),                             // Default pod limit
	}

	// Build requirements
	requirements := scheduling.NewRequirements(
		// Instance type
		scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, offering.Name),
		// Zone
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
		// Capacity type - CloudStack only supports on-demand
		scheduling.NewRequirement(v1.LabelCapacityType, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
		// Architecture - assume amd64 unless specified
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, v1.ArchitectureAmd64),
		// OS
		scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, v1.OSLinux),
	)

	// Create offerings - CloudStack only has on-demand
	offerings := cloudprovider.Offerings{
		{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
				scheduling.NewRequirement(v1.LabelCapacityType, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
			),
			Price:     calculatePrice(offering), // Simple pricing calculation
			Available: true,
		},
	}

	return &cloudprovider.InstanceType{
		Name:         offering.Name,
		Requirements: requirements,
		Offerings:    offerings,
		Capacity:     capacity,
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	}
}

// calculatePrice calculates a simple price for the service offering
// This is a basic implementation - you may want to integrate with actual pricing
func calculatePrice(offering *cloudstack.ServiceOffering) float64 {
	// Simple calculation: $0.04 per vCPU + $0.005 per GB RAM per hour
	cpuCost := float64(offering.Cpunumber) * 0.04
	memCost := float64(offering.Memory) / 1024.0 * 0.005
	return cpuCost + memCost
}
