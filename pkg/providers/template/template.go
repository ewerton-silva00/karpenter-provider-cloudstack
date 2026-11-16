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

package template

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

// Provider provides template information
type Provider interface {
	List(ctx context.Context, zone string) ([]*Template, error)
	ResolveTemplates(ctx context.Context, terms []v1.TemplateSelectorTerm, zone string) ([]*Template, error)
}

// Template represents a CloudStack template
type Template struct {
	ID          string
	Name        string
	DisplayText string
	Zone        string
	ZoneID      string
	OSType      string
	OSTypeName  string
	Status      string
	IsReady     bool
	IsPublic    bool
	IsFeatured  bool
	Tags        map[string]string
}

// DefaultProvider implements the Template Provider
type DefaultProvider struct {
	csClient csapi.CloudStackAPI
	cache    *cache.Cache
	mu       sync.RWMutex
}

// NewDefaultProvider creates a new template provider
func NewDefaultProvider(csClient csapi.CloudStackAPI, cache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		csClient: csClient,
		cache:    cache,
	}
}

// List returns all templates in a zone
func (p *DefaultProvider) List(ctx context.Context, zone string) ([]*Template, error) {
	cacheKey := fmt.Sprintf("templates-%s", zone)

	// Check cache first
	if cached, found := p.cache.Get(cacheKey); found {
		return cached.([]*Template), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock
	if cached, found := p.cache.Get(cacheKey); found {
		return cached.([]*Template), nil
	}

	// Get zone ID
	zoneID, _, err := p.csClient.(*csapi.Client).Zone.GetZoneID(zone)
	if err != nil {
		return nil, fmt.Errorf("getting zone ID for %s: %w", zone, err)
	}

	// Fetch templates from CloudStack
	// List featured, community, and self templates
	var allTemplates []*Template

	for _, templateFilter := range []string{"featured", "community", "self"} {
		params := p.csClient.(*csapi.Client).Template.NewListTemplatesParams(templateFilter)
		params.SetZoneid(zoneID)
		params.SetTemplatefilter(templateFilter)

		resp, err := p.csClient.ListTemplates(params)
		if err != nil {
			log.FromContext(ctx).V(1).Info("Failed to list templates", "filter", templateFilter, "error", err)
			continue
		}

		for _, csTemplate := range resp.Templates {
			// Fetch tags for this template
			tags, err := p.getTemplateTags(ctx, csTemplate.Id)
			if err != nil {
				log.FromContext(ctx).V(1).Info("Failed to get tags for template", "template", csTemplate.Id, "error", err)
				tags = make(map[string]string)
			}

			template := &Template{
				ID:          csTemplate.Id,
				Name:        csTemplate.Name,
				DisplayText: csTemplate.Displaytext,
				Zone:        csTemplate.Zonename,
				ZoneID:      csTemplate.Zoneid,
				OSType:      csTemplate.Ostypeid,
				OSTypeName:  csTemplate.Ostypename,
				Status:      csTemplate.Status,
				IsReady:     csTemplate.Isready,
				IsPublic:    csTemplate.Ispublic,
				IsFeatured:  csTemplate.Isfeatured,
				Tags:        tags,
			}
			allTemplates = append(allTemplates, template)
		}
	}

	// Remove duplicates by ID
	allTemplates = lo.UniqBy(allTemplates, func(t *Template) string {
		return t.ID
	})

	// Cache the results
	p.cache.Set(cacheKey, allTemplates, cache.DefaultExpiration)

	log.FromContext(ctx).Info("Listed templates", "zone", zone, "count", len(allTemplates))

	return allTemplates, nil
}

// ResolveTemplates resolves templates based on selector terms
func (p *DefaultProvider) ResolveTemplates(ctx context.Context, terms []v1.TemplateSelectorTerm, zone string) ([]*Template, error) {
	allTemplates, err := p.List(ctx, zone)
	if err != nil {
		return nil, err
	}

	var matchedTemplates []*Template

	for _, term := range terms {
		// Match by ID (highest priority)
		if term.ID != "" {
			template, found := lo.Find(allTemplates, func(t *Template) bool {
				return t.ID == term.ID
			})
			if found {
				matchedTemplates = append(matchedTemplates, template)
				continue
			}
		}

		// Match by Name
		if term.Name != "" {
			template, found := lo.Find(allTemplates, func(t *Template) bool {
				return t.Name == term.Name
			})
			if found {
				matchedTemplates = append(matchedTemplates, template)
				continue
			}
		}

		// Filter by OSType if specified
		templates := allTemplates
		if term.OSType != "" {
			templates = lo.Filter(templates, func(t *Template, _ int) bool {
				return t.OSTypeName == term.OSType || t.OSType == term.OSType
			})
		}

		// Match by Tags
		if len(term.Tags) > 0 {
			matches := lo.Filter(templates, func(t *Template, _ int) bool {
				return matchesTags(t.Tags, term.Tags)
			})
			matchedTemplates = append(matchedTemplates, matches...)
		} else if term.OSType != "" {
			// If only OSType is specified, add all matching templates
			matchedTemplates = append(matchedTemplates, templates...)
		}
	}

	// Remove duplicates
	matchedTemplates = lo.UniqBy(matchedTemplates, func(t *Template) string {
		return t.ID
	})

	// Filter only ready templates
	matchedTemplates = lo.Filter(matchedTemplates, func(t *Template, _ int) bool {
		return t.IsReady && t.Status == "Download Complete"
	})

	if len(matchedTemplates) == 0 {
		return nil, fmt.Errorf("no templates matched the selector terms in zone %s", zone)
	}

	log.FromContext(ctx).Info("Resolved templates", "zone", zone, "count", len(matchedTemplates))

	return matchedTemplates, nil
}

// getTemplateTags fetches tags for a template
func (p *DefaultProvider) getTemplateTags(ctx context.Context, templateID string) (map[string]string, error) {
	params := p.csClient.(*csapi.Client).Resourcetags.NewListTagsParams()
	params.SetResourceid(templateID)
	params.SetResourcetype("Template")

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
