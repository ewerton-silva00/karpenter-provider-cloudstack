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

package operator

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter/pkg/operator"

	csapi "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/cloudstack"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/operator/options"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/instance"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/instancetype"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/network"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/template"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/zone"
)

const (
	defaultCacheTTL        = 15 * time.Minute
	defaultCleanupInterval = 30 * time.Minute
)

// Operator is the CloudStack-specific operator
type Operator struct {
	*operator.Operator

	CloudStackClient     csapi.CloudStackAPI
	ZoneProvider         zone.Provider
	NetworkProvider      network.Provider
	TemplateProvider     template.Provider
	InstanceTypeProvider instancetype.Provider
	InstanceProvider     instance.Provider
}

// NewOperator creates a new CloudStack operator
func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	// Parse options
	opts := &options.Options{}
	if err := opts.Parse(ctx); err != nil {
		log.FromContext(ctx).Error(err, "Failed to parse options")
		panic(err)
	}
	ctx = options.ToContext(ctx, opts)

	// Create CloudStack client
	csClient, err := csapi.NewClient(ctx, csapi.Config{
		APIURL:    opts.CloudStackAPIURL,
		APIKey:    opts.CloudStackAPIKey,
		SecretKey: opts.CloudStackSecretKey,
		VerifySSL: opts.CloudStackVerifySSL,
		Timeout:   60 * time.Second,
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to create CloudStack client")
		panic(err)
	}

	// Create caches
	zoneCache := cache.New(defaultCacheTTL, defaultCleanupInterval)
	networkCache := cache.New(defaultCacheTTL, defaultCleanupInterval)
	templateCache := cache.New(defaultCacheTTL, defaultCleanupInterval)
	instanceTypeCache := cache.New(defaultCacheTTL, defaultCleanupInterval)
	instanceCache := cache.New(defaultCacheTTL, defaultCleanupInterval)

	// Create providers
	zoneProvider := zone.NewDefaultProvider(csClient, zoneCache)
	networkProvider := network.NewDefaultProvider(csClient, networkCache)
	templateProvider := template.NewDefaultProvider(csClient, templateCache)
	instanceTypeProvider := instancetype.NewDefaultProvider(csClient, instanceTypeCache)
	instanceProvider := instance.NewDefaultProvider(
		csClient,
		networkProvider,
		templateProvider,
		instanceCache,
		opts.ClusterName,
	)

	log.FromContext(ctx).Info("CloudStack operator initialized successfully")

	return ctx, &Operator{
		Operator:             operator,
		CloudStackClient:     csClient,
		ZoneProvider:         zoneProvider,
		NetworkProvider:      networkProvider,
		TemplateProvider:     templateProvider,
		InstanceTypeProvider: instanceTypeProvider,
		InstanceProvider:     instanceProvider,
	}
}
