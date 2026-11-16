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

package network

import (
	"context"
	"fmt"
	"sync"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/apis/v1"
	csapi "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/cloudstack"
)

// Provider provides network information
type Provider interface {
	List(ctx context.Context, zone string) ([]*Network, error)
	ResolveNetworks(ctx context.Context, terms []v1.NetworkSelectorTerm, zone string) ([]*Network, error)
}

// Network represents a CloudStack network
type Network struct {
	ID      string
	Name    string
	Zone    string
	ZoneID  string
	Type    string
	State   string
	CIDR    string
	Gateway string
	Tags    map[string]string
}

// DefaultProvider implements the Network Provider
type DefaultProvider struct {
	csClient csapi.CloudStackAPI
	cache    *cache.Cache
	mu       sync.RWMutex
}

// NewDefaultProvider creates a new network provider
func NewDefaultProvider(csClient csapi.CloudStackAPI, cache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		csClient: csClient,
		cache:    cache,
	}
}

// List returns all networks in a zone
func (p *DefaultProvider) List(ctx context.Context, zone string) ([]*Network, error) {
	cacheKey := fmt.Sprintf("networks-%s", zone)

	// Check cache first
	if cached, found := p.cache.Get(cacheKey); found {
		return cached.([]*Network), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock
	if cached, found := p.cache.Get(cacheKey); found {
		return cached.([]*Network), nil
	}

	// Get zone ID
	zoneID, _, err := p.csClient.(*csapi.Client).Zone.GetZoneID(zone)
	if err != nil {
		return nil, fmt.Errorf("getting zone ID for %s: %w", zone, err)
	}

	// Fetch networks from CloudStack
	params := p.csClient.(*csapi.Client).Network.NewListNetworksParams()
	params.SetZoneid(zoneID)

	resp, err := p.csClient.ListNetworks(params)
	if err != nil {
		return nil, fmt.Errorf("listing networks in zone %s: %w", zone, err)
	}

	networks := make([]*Network, 0, len(resp.Networks))
	for _, csNet := range resp.Networks {
		// Fetch tags for this network
		tags, err := p.getNetworkTags(ctx, csNet.Id)
		if err != nil {
			log.FromContext(ctx).V(1).Info("Failed to get tags for network", "network", csNet.Id, "error", err)
			tags = make(map[string]string)
		}

		network := &Network{
			ID:      csNet.Id,
			Name:    csNet.Name,
			Zone:    csNet.Zonename,
			ZoneID:  csNet.Zoneid,
			Type:    csNet.Type,
			State:   csNet.State,
			CIDR:    csNet.Cidr,
			Gateway: csNet.Gateway,
			Tags:    tags,
		}
		networks = append(networks, network)
	}

	// Cache the results
	p.cache.Set(cacheKey, networks, cache.DefaultExpiration)

	log.FromContext(ctx).Info("Listed networks", "zone", zone, "count", len(networks))

	return networks, nil
}

// ResolveNetworks resolves networks based on selector terms
func (p *DefaultProvider) ResolveNetworks(ctx context.Context, terms []v1.NetworkSelectorTerm, zone string) ([]*Network, error) {
	allNetworks, err := p.List(ctx, zone)
	if err != nil {
		return nil, err
	}

	var matchedNetworks []*Network

	for _, term := range terms {
		// Match by ID (highest priority)
		if term.ID != "" {
			network, found := lo.Find(allNetworks, func(n *Network) bool {
				return n.ID == term.ID
			})
			if found {
				matchedNetworks = append(matchedNetworks, network)
				continue
			}
		}

		// Match by Name
		if term.Name != "" {
			network, found := lo.Find(allNetworks, func(n *Network) bool {
				return n.Name == term.Name
			})
			if found {
				matchedNetworks = append(matchedNetworks, network)
				continue
			}
		}

		// Match by Tags
		if len(term.Tags) > 0 {
			matches := lo.Filter(allNetworks, func(n *Network, _ int) bool {
				return matchesTags(n.Tags, term.Tags)
			})
			matchedNetworks = append(matchedNetworks, matches...)
		}
	}

	// Remove duplicates
	matchedNetworks = lo.UniqBy(matchedNetworks, func(n *Network) string {
		return n.ID
	})

	// Filter only networks in "Implemented" state
	matchedNetworks = lo.Filter(matchedNetworks, func(n *Network, _ int) bool {
		return n.State == "Implemented" || n.State == "Setup"
	})

	if len(matchedNetworks) == 0 {
		return nil, fmt.Errorf("no networks matched the selector terms in zone %s", zone)
	}

	log.FromContext(ctx).Info("Resolved networks", "zone", zone, "count", len(matchedNetworks))

	return matchedNetworks, nil
}

// getNetworkTags fetches tags for a network
func (p *DefaultProvider) getNetworkTags(ctx context.Context, networkID string) (map[string]string, error) {
	params := p.csClient.(*csapi.Client).Resourcetags.NewListTagsParams()
	params.SetResourceid(networkID)
	params.SetResourcetype("Network")

	resp, err := p.csClient.ListTags(params)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for _, tag := range resp.Tags {
		tags[tag.Key] = tag.Value
	}

	return tags, nil
}

// matchesTags checks if resource tags match selector tags
// Supports wildcard matching with '*'
func matchesTags(resourceTags, selectorTags map[string]string) bool {
	for key, value := range selectorTags {
		resourceValue, exists := resourceTags[key]
		if !exists {
			return false
		}
		// Support wildcard
		if value != "*" && resourceValue != value {
			return false
		}
	}
	return true
}
