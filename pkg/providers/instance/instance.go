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
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/patrickmn/go-cache"
	"sigs.k8s.io/controller-runtime/pkg/log"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"

	v1 "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/apis/v1"
	csapi "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/cloudstack"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/network"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/template"
)

// Provider provides instance management
type Provider interface {
	Create(ctx context.Context, nodeClass *v1.CloudStackNodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*Instance, error)
	Get(ctx context.Context, id string) (*Instance, error)
	List(ctx context.Context) ([]*Instance, error)
	Delete(ctx context.Context, id string) error
	DetachISO(ctx context.Context, id string) error
}

// vmNamePrefix prefixes the name of every VM launched by Karpenter. The VM name
// becomes the hostname, and therefore the name of the Node.
const vmNamePrefix = "karpenter-"

// VMName returns the name of the VM launched for a NodeClaim.
func VMName(nodeClaimName string) string {
	return vmNamePrefix + nodeClaimName
}

// NodeClaimNameFromVMName returns the name of the NodeClaim a VM was launched
// for, and false if the name does not belong to a VM launched by Karpenter.
func NodeClaimNameFromVMName(vmName string) (string, bool) {
	name, ok := strings.CutPrefix(vmName, vmNamePrefix)
	return name, ok && name != ""
}

// Instance represents a CloudStack virtual machine
type Instance struct {
	ID                string
	Name              string
	State             string
	Zone              string
	ZoneID            string
	ServiceOffering   string
	ServiceOfferingID string
	Template          string
	TemplateID        string
	NetworkID         string
	IPAddress         string
	CreatedTime       time.Time
	Tags              map[string]string
}

// DefaultProvider implements the Instance Provider
type DefaultProvider struct {
	csClient         csapi.CloudStackAPI
	networkProvider  network.Provider
	templateProvider template.Provider
	cache            *cache.Cache
	clusterName      string
}

// NewDefaultProvider creates a new instance provider
func NewDefaultProvider(
	csClient csapi.CloudStackAPI,
	networkProvider network.Provider,
	templateProvider template.Provider,
	cache *cache.Cache,
	clusterName string,
) *DefaultProvider {
	return &DefaultProvider{
		csClient:         csClient,
		networkProvider:  networkProvider,
		templateProvider: templateProvider,
		cache:            cache,
		clusterName:      clusterName,
	}
}

// Create creates a new virtual machine
func (p *DefaultProvider) Create(ctx context.Context, nodeClass *v1.CloudStackNodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*Instance, error) {
	log.FromContext(ctx).Info("Creating instance", "nodeClaim", nodeClaim.Name)

	// Select instance type (service offering)
	instanceType := selectInstanceType(nodeClaim, instanceTypes)
	if instanceType == nil {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("no instance type satisfies the requirements and resource requests of the nodeclaim"))
	}

	// Get zone ID
	zoneID, _, err := p.csClient.(*csapi.Client).Zone.GetZoneID(nodeClass.Spec.Zone)
	if err != nil {
		return nil, fmt.Errorf("getting zone ID: %w", err)
	}

	// Resolve network
	networks, err := p.networkProvider.ResolveNetworks(ctx, nodeClass.Spec.NetworkSelectorTerms, nodeClass.Spec.Zone)
	if err != nil {
		return nil, fmt.Errorf("resolving networks: %w", err)
	}
	if len(networks) == 0 {
		return nil, fmt.Errorf("no networks found")
	}
	networkID := networks[0].ID

	// Resolve template
	templates, err := p.templateProvider.ResolveTemplates(ctx, nodeClass.Spec.TemplateSelectorTerms, nodeClass.Spec.Zone)
	if err != nil {
		return nil, fmt.Errorf("resolving templates: %w", err)
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("no templates found")
	}
	templateID := templates[0].ID

	// Get service offering ID
	serviceOfferingID, _, err := p.csClient.(*csapi.Client).ServiceOffering.GetServiceOfferingID(instanceType.Name)
	if err != nil {
		return nil, fmt.Errorf("getting service offering ID: %w", err)
	}

	// Prepare deploy parameters
	deployParams := p.csClient.(*csapi.Client).VirtualMachine.NewDeployVirtualMachineParams(
		serviceOfferingID,
		templateID,
		zoneID,
	)

	// Set network
	deployParams.SetNetworkids([]string{networkID})

	// Set name
	vmName := VMName(nodeClaim.Name)
	deployParams.SetName(vmName)
	deployParams.SetDisplayname(vmName)

	// Set user data if provided
	if nodeClass.Spec.UserData != nil && *nodeClass.Spec.UserData != "" {
		userData := base64.StdEncoding.EncodeToString([]byte(*nodeClass.Spec.UserData))
		deployParams.SetUserdata(userData)
	}

	// Set root disk size if specified
	if nodeClass.Spec.RootDiskSize != nil {
		deployParams.SetRootdisksize(*nodeClass.Spec.RootDiskSize)
	}

	// Set SSH key pair if specified
	if nodeClass.Spec.SSHKeyPair != nil {
		deployParams.SetKeypair(*nodeClass.Spec.SSHKeyPair)
	}

	// Resolve the ISO before deploying, so a bad selector does not leave a VM behind
	isoID, err := p.resolveISO(nodeClass, zoneID)
	if err != nil {
		return nil, fmt.Errorf("resolving ISO: %w", err)
	}

	// Deploy the VM
	resp, err := p.csClient.DeployVirtualMachine(deployParams)
	if err != nil {
		// The VM may exist even though the deployment failed, e.g. when it could not be started
		p.destroyByName(ctx, vmName)
		return nil, fmt.Errorf("deploying virtual machine: %w", err)
	}

	// From here on the VM exists: destroy it if anything fails, otherwise it
	// would be left running without a NodeClaim
	instance, err := p.finishCreate(ctx, resp.Id, isoID, nodeClass, nodeClaim)
	if err != nil {
		p.destroy(ctx, resp.Id)
		return nil, err
	}

	log.FromContext(ctx).Info("Instance created successfully", "instanceID", instance.ID, "name", instance.Name, "instanceType", instanceType.Name)

	return instance, nil
}

// finishCreate tags a deployed VM, waits for it to run and attaches the ISO
func (p *DefaultProvider) finishCreate(ctx context.Context, vmID, isoID string, nodeClass *v1.CloudStackNodeClass, nodeClaim *karpv1.NodeClaim) (*Instance, error) {
	// Tag first: List only returns tagged VMs, so an untagged VM is invisible to garbage collection
	tags := p.buildTags(nodeClass, nodeClaim)
	if err := p.createTags(ctx, vmID, tags); err != nil {
		return nil, fmt.Errorf("creating tags: %w", err)
	}

	vm, err := p.waitForVMState(ctx, vmID, "Running", 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("waiting for VM to be running: %w", err)
	}

	if isoID != "" {
		iso := p.csClient.(*csapi.Client).ISO
		if _, err := iso.AttachIso(iso.NewAttachIsoParams(isoID, vmID)); err != nil {
			return nil, fmt.Errorf("attaching ISO %s: %w", isoID, err)
		}
	}

	return p.convertToInstance(vm, tags), nil
}

// resolveISO returns the ID of the ISO selected by the node class, or "" if none is
func (p *DefaultProvider) resolveISO(nodeClass *v1.CloudStackNodeClass, zoneID string) (string, error) {
	if nodeClass.Spec.ISO == nil {
		return "", nil
	}
	if nodeClass.Spec.ISO.ID != "" {
		return nodeClass.Spec.ISO.ID, nil
	}
	id, _, err := p.csClient.(*csapi.Client).ISO.GetIsoID(nodeClass.Spec.ISO.Name, "executable", zoneID)
	return id, err
}

// DetachISO detaches the ISO from a VM, if one is attached
func (p *DefaultProvider) DetachISO(ctx context.Context, id string) error {
	params := p.csClient.(*csapi.Client).VirtualMachine.NewListVirtualMachinesParams()
	params.SetId(id)
	resp, err := p.csClient.ListVirtualMachines(params)
	if err != nil {
		return fmt.Errorf("getting instance %s: %w", id, err)
	}
	if resp.Count == 0 || resp.VirtualMachines[0].Isoid == "" {
		return nil
	}

	iso := p.csClient.(*csapi.Client).ISO
	if _, err := iso.DetachIso(iso.NewDetachIsoParams(id)); err != nil {
		return fmt.Errorf("detaching ISO from instance %s: %w", id, err)
	}
	return nil
}

// destroy destroys and expunges a VM, logging instead of returning the error
func (p *DefaultProvider) destroy(ctx context.Context, id string) {
	params := p.csClient.(*csapi.Client).VirtualMachine.NewDestroyVirtualMachineParams(id)
	params.SetExpunge(true)
	if _, err := p.csClient.DestroyVirtualMachine(params); err != nil {
		log.FromContext(ctx).Error(err, "Failed to destroy instance, it must be removed manually", "instanceID", id)
	}
}

// destroyByName destroys the VMs left behind by a failed deployment
func (p *DefaultProvider) destroyByName(ctx context.Context, name string) {
	params := p.csClient.(*csapi.Client).VirtualMachine.NewListVirtualMachinesParams()
	params.SetName(name)
	resp, err := p.csClient.ListVirtualMachines(params)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to look for a leftover instance", "name", name)
		return
	}
	for _, vm := range resp.VirtualMachines {
		// The name filter matches substrings
		if vm.Name == name {
			p.destroy(ctx, vm.Id)
		}
	}
}

// Get retrieves an instance by ID
func (p *DefaultProvider) Get(ctx context.Context, id string) (*Instance, error) {
	// Check cache
	cacheKey := fmt.Sprintf("instance-%s", id)
	if cached, found := p.cache.Get(cacheKey); found {
		return cached.(*Instance), nil
	}

	params := p.csClient.(*csapi.Client).VirtualMachine.NewListVirtualMachinesParams()
	params.SetId(id)

	resp, err := p.csClient.ListVirtualMachines(params)
	if err != nil {
		return nil, fmt.Errorf("getting instance %s: %w", id, err)
	}

	if resp.Count == 0 {
		return nil, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("instance %s not found", id))
	}

	vm := resp.VirtualMachines[0]
	tags, _ := p.getTags(ctx, vm.Id)
	instance := p.convertToInstance(vm, tags)

	// Cache the result
	p.cache.Set(cacheKey, instance, cache.DefaultExpiration)

	return instance, nil
}

// List lists all instances managed by Karpenter
func (p *DefaultProvider) List(ctx context.Context) ([]*Instance, error) {
	// List VMs with Karpenter tags
	params := p.csClient.(*csapi.Client).VirtualMachine.NewListVirtualMachinesParams()

	resp, err := p.csClient.ListVirtualMachines(params)
	if err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}

	instances := make([]*Instance, 0)
	for _, vm := range resp.VirtualMachines {
		// Get tags to check if this is a Karpenter-managed VM
		tags, err := p.getTags(ctx, vm.Id)
		if err != nil {
			log.FromContext(ctx).V(1).Info("Failed to get tags for VM", "vmID", vm.Id, "error", err)
			continue
		}

		// Only include VMs with Karpenter tags
		if _, hasTag := tags[v1.ManagedByTagKey]; hasTag {
			instance := p.convertToInstance(vm, tags)
			instances = append(instances, instance)
		}
	}

	log.FromContext(ctx).Info("Listed instances", "count", len(instances))

	return instances, nil
}

// Delete deletes an instance
func (p *DefaultProvider) Delete(ctx context.Context, id string) error {
	log.FromContext(ctx).Info("Deleting instance", "instanceID", id)

	// Check the current state, not a cached one
	p.cache.Delete(fmt.Sprintf("instance-%s", id))
	inst, err := p.Get(ctx, id)
	if err != nil {
		if cloudprovider.IsNodeClaimNotFoundError(err) {
			return err
		}
		return fmt.Errorf("checking instance existence: %w", err)
	}
	p.cache.Delete(fmt.Sprintf("instance-%s", id))
	if inst.State == "Destroyed" || inst.State == "Expunging" {
		return cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("instance %s is %s", id, inst.State))
	}

	// Destroy the VM
	params := p.csClient.(*csapi.Client).VirtualMachine.NewDestroyVirtualMachineParams(id)
	params.SetExpunge(true)

	_, err = p.csClient.DestroyVirtualMachine(params)
	if err != nil {
		return fmt.Errorf("destroying instance %s: %w", id, err)
	}

	log.FromContext(ctx).Info("Instance deleted successfully", "instanceID", id)

	return nil
}

// selectInstanceType returns the cheapest instance type that satisfies the
// requirements and fits the resource requests of the node claim, or nil if
// there is none. Ties are broken by name so the choice is deterministic.
func selectInstanceType(nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) *cloudprovider.InstanceType {
	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)

	var best *cloudprovider.InstanceType
	bestPrice := math.MaxFloat64
	for _, it := range instanceTypes {
		if !reqs.IsCompatible(it.Requirements, scheduling.AllowUndefinedWellKnownLabels) {
			continue
		}
		if !resources.Fits(nodeClaim.Spec.Resources.Requests, it.Allocatable()) {
			continue
		}
		offerings := it.Offerings.Available().Compatible(reqs)
		if len(offerings) == 0 {
			continue
		}
		price := offerings.Cheapest().Price
		if price < bestPrice || (price == bestPrice && it.Name < best.Name) {
			best, bestPrice = it, price
		}
	}
	return best
}

// waitForVMState waits for a VM to reach a specific state
func (p *DefaultProvider) waitForVMState(ctx context.Context, vmID, targetState string, timeout time.Duration) (*cloudstack.VirtualMachine, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutChan:
			return nil, fmt.Errorf("timeout waiting for VM %s to reach state %s", vmID, targetState)
		case <-ticker.C:
			params := p.csClient.(*csapi.Client).VirtualMachine.NewListVirtualMachinesParams()
			params.SetId(vmID)

			resp, err := p.csClient.ListVirtualMachines(params)
			if err != nil {
				return nil, fmt.Errorf("querying VM state: %w", err)
			}

			if resp.Count == 0 {
				return nil, fmt.Errorf("VM %s not found", vmID)
			}

			vm := resp.VirtualMachines[0]
			if vm.State == targetState {
				return vm, nil
			}
		}
	}
}

// buildTags builds tags for the VM
func (p *DefaultProvider) buildTags(nodeClass *v1.CloudStackNodeClass, nodeClaim *karpv1.NodeClaim) map[string]string {
	tags := map[string]string{
		v1.ManagedByTagKey:                         "karpenter",
		v1.ClusterNameTagKey + "/" + p.clusterName: "owned",
		v1.NodeClassTagKey:                         nodeClass.Name,
		v1.NodeClaimTagKey:                         nodeClaim.Name,
	}

	// Add nodepool tag if present
	if nodePoolName, ok := nodeClaim.Labels[karpv1.NodePoolLabelKey]; ok {
		tags[v1.NodePoolTagKey] = nodePoolName
	}

	// Add user-defined tags
	for k, v := range nodeClass.Spec.Tags {
		tags[k] = v
	}

	return tags
}

// createTags creates tags for a resource
func (p *DefaultProvider) createTags(ctx context.Context, resourceID string, tags map[string]string) error {
	params := p.csClient.(*csapi.Client).Resourcetags.NewCreateTagsParams(
		[]string{resourceID},
		"UserVm",
		tags,
	)

	_, err := p.csClient.CreateTags(params)
	return err
}

// getTags fetches tags for a resource
func (p *DefaultProvider) getTags(ctx context.Context, resourceID string) (map[string]string, error) {
	params := p.csClient.(*csapi.Client).Resourcetags.NewListTagsParams()
	params.SetResourceid(resourceID)
	params.SetResourcetype("UserVm")

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

// convertToInstance converts a CloudStack VM to an Instance
func (p *DefaultProvider) convertToInstance(vm *cloudstack.VirtualMachine, tags map[string]string) *Instance {
	// Parse creation time from CloudStack date string
	createdTime := parseCloudStackTime(vm.Created)

	return &Instance{
		ID:                vm.Id,
		Name:              vm.Name,
		State:             vm.State,
		Zone:              vm.Zonename,
		ZoneID:            vm.Zoneid,
		ServiceOffering:   vm.Serviceofferingname,
		ServiceOfferingID: vm.Serviceofferingid,
		Template:          vm.Templatename,
		TemplateID:        vm.Templateid,
		NetworkID:         getFirstNetworkID(vm.Nic),
		IPAddress:         getFirstIPAddress(vm.Nic),
		CreatedTime:       createdTime,
		Tags:              tags,
	}
}

// parseCloudStackTime parses CloudStack time format (ISO 8601) to time.Time
func parseCloudStackTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Time{}
	}

	// CloudStack uses ISO 8601 format: "2006-01-02T15:04:05-0700"
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05+0000",
		"2006-01-02T15:04:05Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	// If parsing fails, return zero time
	return time.Time{}
}

// getFirstNetworkID returns the first network ID from NICs
func getFirstNetworkID(nics []cloudstack.Nic) string {
	if len(nics) > 0 {
		return nics[0].Networkid
	}
	return ""
}

// getFirstIPAddress returns the first IP address from NICs
func getFirstIPAddress(nics []cloudstack.Nic) string {
	if len(nics) > 0 {
		return nics[0].Ipaddress
	}
	return ""
}
