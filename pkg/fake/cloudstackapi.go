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

package fake

import (
	"github.com/apache/cloudstack-go/v2/cloudstack"
	csapi "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/cloudstack"
)

// CloudStackAPI is a fake implementation of the CloudStack API for testing
type CloudStackAPI struct {
	// VirtualMachine responses
	DeployVirtualMachineFunc  func(*cloudstack.DeployVirtualMachineParams) (*cloudstack.DeployVirtualMachineResponse, error)
	ListVirtualMachinesFunc   func(*cloudstack.ListVirtualMachinesParams) (*cloudstack.ListVirtualMachinesResponse, error)
	DestroyVirtualMachineFunc func(*cloudstack.DestroyVirtualMachineParams) (*cloudstack.DestroyVirtualMachineResponse, error)

	// ServiceOffering responses
	ListServiceOfferingsFunc func(*cloudstack.ListServiceOfferingsParams) (*cloudstack.ListServiceOfferingsResponse, error)

	// Template responses
	ListTemplatesFunc func(*cloudstack.ListTemplatesParams) (*cloudstack.ListTemplatesResponse, error)

	// Network responses
	ListNetworksFunc func(*cloudstack.ListNetworksParams) (*cloudstack.ListNetworksResponse, error)

	// Zone responses
	ListZonesFunc func(*cloudstack.ListZonesParams) (*cloudstack.ListZonesResponse, error)

	// Tag responses
	CreateTagsFunc func(*cloudstack.CreateTagsParams) (*cloudstack.CreateTagsResponse, error)
	ListTagsFunc   func(*cloudstack.ListTagsParams) (*cloudstack.ListTagsResponse, error)
}

var _ csapi.CloudStackAPI = (*CloudStackAPI)(nil)

func (f *CloudStackAPI) DeployVirtualMachine(p *cloudstack.DeployVirtualMachineParams) (*cloudstack.DeployVirtualMachineResponse, error) {
	if f.DeployVirtualMachineFunc != nil {
		return f.DeployVirtualMachineFunc(p)
	}
	return &cloudstack.DeployVirtualMachineResponse{}, nil
}

func (f *CloudStackAPI) GetVirtualMachineID(name string, opts ...cloudstack.OptionFunc) (string, int, error) {
	return "vm-123", 1, nil
}

func (f *CloudStackAPI) ListVirtualMachines(p *cloudstack.ListVirtualMachinesParams) (*cloudstack.ListVirtualMachinesResponse, error) {
	if f.ListVirtualMachinesFunc != nil {
		return f.ListVirtualMachinesFunc(p)
	}
	return &cloudstack.ListVirtualMachinesResponse{}, nil
}

func (f *CloudStackAPI) DestroyVirtualMachine(p *cloudstack.DestroyVirtualMachineParams) (*cloudstack.DestroyVirtualMachineResponse, error) {
	if f.DestroyVirtualMachineFunc != nil {
		return f.DestroyVirtualMachineFunc(p)
	}
	return &cloudstack.DestroyVirtualMachineResponse{}, nil
}

func (f *CloudStackAPI) ListServiceOfferings(p *cloudstack.ListServiceOfferingsParams) (*cloudstack.ListServiceOfferingsResponse, error) {
	if f.ListServiceOfferingsFunc != nil {
		return f.ListServiceOfferingsFunc(p)
	}
	return &cloudstack.ListServiceOfferingsResponse{}, nil
}

func (f *CloudStackAPI) GetServiceOfferingID(name string, opts ...cloudstack.OptionFunc) (string, int, error) {
	return "offering-123", 1, nil
}

func (f *CloudStackAPI) ListTemplates(p *cloudstack.ListTemplatesParams) (*cloudstack.ListTemplatesResponse, error) {
	if f.ListTemplatesFunc != nil {
		return f.ListTemplatesFunc(p)
	}
	return &cloudstack.ListTemplatesResponse{}, nil
}

func (f *CloudStackAPI) GetTemplateID(name string, filter string, zoneid string, opts ...cloudstack.OptionFunc) (string, int, error) {
	return "template-123", 1, nil
}

func (f *CloudStackAPI) ListNetworks(p *cloudstack.ListNetworksParams) (*cloudstack.ListNetworksResponse, error) {
	if f.ListNetworksFunc != nil {
		return f.ListNetworksFunc(p)
	}
	return &cloudstack.ListNetworksResponse{}, nil
}

func (f *CloudStackAPI) GetNetworkID(name string, opts ...cloudstack.OptionFunc) (string, int, error) {
	return "network-123", 1, nil
}

func (f *CloudStackAPI) ListZones(p *cloudstack.ListZonesParams) (*cloudstack.ListZonesResponse, error) {
	if f.ListZonesFunc != nil {
		return f.ListZonesFunc(p)
	}
	return &cloudstack.ListZonesResponse{}, nil
}

func (f *CloudStackAPI) GetZoneID(name string, opts ...cloudstack.OptionFunc) (string, int, error) {
	return "zone-123", 1, nil
}

func (f *CloudStackAPI) ListDiskOfferings(p *cloudstack.ListDiskOfferingsParams) (*cloudstack.ListDiskOfferingsResponse, error) {
	return &cloudstack.ListDiskOfferingsResponse{}, nil
}

func (f *CloudStackAPI) GetDiskOfferingID(name string, opts ...cloudstack.OptionFunc) (string, int, error) {
	return "disk-offering-123", 1, nil
}

func (f *CloudStackAPI) CreateTags(p *cloudstack.CreateTagsParams) (*cloudstack.CreateTagsResponse, error) {
	if f.CreateTagsFunc != nil {
		return f.CreateTagsFunc(p)
	}
	return &cloudstack.CreateTagsResponse{}, nil
}

func (f *CloudStackAPI) DeleteTags(p *cloudstack.DeleteTagsParams) (*cloudstack.DeleteTagsResponse, error) {
	return &cloudstack.DeleteTagsResponse{}, nil
}

func (f *CloudStackAPI) ListTags(p *cloudstack.ListTagsParams) (*cloudstack.ListTagsResponse, error) {
	if f.ListTagsFunc != nil {
		return f.ListTagsFunc(p)
	}
	return &cloudstack.ListTagsResponse{}, nil
}
