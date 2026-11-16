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

package zone

import (
	"context"
	"fmt"
	"sync"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/log"

	csapi "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/cloudstack"
)

// Provider provides zone information
type Provider interface {
	List(ctx context.Context) ([]*Zone, error)
	Get(ctx context.Context, id string) (*Zone, error)
	GetByName(ctx context.Context, name string) (*Zone, error)
}

// Zone represents a CloudStack zone
type Zone struct {
	ID                    string
	Name                  string
	NetworkType           string
	AllocationState       string
	LocalStorageEnabled   bool
	SecurityGroupsEnabled bool
}

// DefaultProvider implements the Zone Provider
type DefaultProvider struct {
	csClient csapi.CloudStackAPI
	cache    *cache.Cache
	mu       sync.RWMutex
}

// NewDefaultProvider creates a new zone provider
func NewDefaultProvider(csClient csapi.CloudStackAPI, cache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		csClient: csClient,
		cache:    cache,
	}
}

// List returns all available zones
func (p *DefaultProvider) List(ctx context.Context) ([]*Zone, error) {
	// Check cache first
	if cached, found := p.cache.Get("zones"); found {
		return cached.([]*Zone), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock
	if cached, found := p.cache.Get("zones"); found {
		return cached.([]*Zone), nil
	}

	// Fetch zones from CloudStack
	params := p.csClient.(*csapi.Client).Zone.NewListZonesParams()
	params.SetAvailable(true)

	resp, err := p.csClient.ListZones(params)
	if err != nil {
		return nil, fmt.Errorf("listing zones: %w", err)
	}

	zones := make([]*Zone, 0, len(resp.Zones))
	for _, csZone := range resp.Zones {
		zone := &Zone{
			ID:                    csZone.Id,
			Name:                  csZone.Name,
			NetworkType:           csZone.Networktype,
			AllocationState:       csZone.Allocationstate,
			LocalStorageEnabled:   csZone.Localstorageenabled,
			SecurityGroupsEnabled: csZone.Securitygroupsenabled,
		}
		zones = append(zones, zone)
	}

	// Cache the results
	p.cache.Set("zones", zones, cache.DefaultExpiration)

	log.FromContext(ctx).Info("Listed zones", "count", len(zones))

	return zones, nil
}

// Get returns a zone by ID
func (p *DefaultProvider) Get(ctx context.Context, id string) (*Zone, error) {
	zones, err := p.List(ctx)
	if err != nil {
		return nil, err
	}

	zone, found := lo.Find(zones, func(z *Zone) bool {
		return z.ID == id
	})

	if !found {
		return nil, fmt.Errorf("zone %s not found", id)
	}

	return zone, nil
}

// GetByName returns a zone by name
func (p *DefaultProvider) GetByName(ctx context.Context, name string) (*Zone, error) {
	zones, err := p.List(ctx)
	if err != nil {
		return nil, err
	}

	zone, found := lo.Find(zones, func(z *Zone) bool {
		return z.Name == name
	})

	if !found {
		return nil, fmt.Errorf("zone %s not found", name)
	}

	return zone, nil
}

// ValidateZone validates that a zone exists and is available
func (p *DefaultProvider) ValidateZone(ctx context.Context, zoneIdentifier string) error {
	zones, err := p.List(ctx)
	if err != nil {
		return fmt.Errorf("validating zone: %w", err)
	}

	zone, found := lo.Find(zones, func(z *Zone) bool {
		return z.ID == zoneIdentifier || z.Name == zoneIdentifier
	})

	if !found {
		return fmt.Errorf("zone %s not found", zoneIdentifier)
	}

	if zone.AllocationState != "Enabled" {
		return fmt.Errorf("zone %s is not enabled (state: %s)", zoneIdentifier, zone.AllocationState)
	}

	return nil
}
