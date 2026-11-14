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

package v1

import (
	"fmt"

	"github.com/awslabs/operatorpkg/status"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudStackNodeClassSpec is the top level specification for the CloudStack Karpenter Provider.
// This will contain configuration necessary to launch instances in CloudStack.
type CloudStackNodeClassSpec struct {
	// Zone is the CloudStack zone where VMs will be launched
	// +kubebuilder:validation:Required
	// +required
	Zone string `json:"zone"`

	// NetworkSelectorTerms is a list of network selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="networkSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['tags', 'id', 'name']",rule="self.all(x, has(x.tags) || has(x.id) || has(x.name))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	NetworkSelectorTerms []NetworkSelectorTerm `json:"networkSelectorTerms"`

	// ServiceOfferingSelectorTerms is a list of service offering selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="serviceOfferingSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['tags', 'id', 'name']",rule="self.all(x, has(x.tags) || has(x.id) || has(x.name))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	ServiceOfferingSelectorTerms []ServiceOfferingSelectorTerm `json:"serviceOfferingSelectorTerms"`

	// TemplateSelectorTerms is a list of template selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="templateSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['tags', 'id', 'name']",rule="self.all(x, has(x.tags) || has(x.id) || has(x.name))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	TemplateSelectorTerms []TemplateSelectorTerm `json:"templateSelectorTerms"`

	// UserData to be applied to the provisioned nodes.
	// It must be in cloud-init format.
	// +optional
	UserData *string `json:"userData,omitempty"`

	// ISO selects an ISO that is attached to each VM right after it is deployed.
	// CloudStack Kubernetes Service nodes install kubeadm, kubelet and the
	// container images from the Kubernetes binaries ISO of the cluster version.
	// +kubebuilder:validation:XValidation:message="expected exactly one of ['id', 'name']",rule="has(self.id) != has(self.name)"
	// +optional
	ISO *ISOSelector `json:"iso,omitempty"`

	// Tags to be applied on CloudStack resources like instances.
	// +kubebuilder:validation:XValidation:message="empty tag keys aren't supported",rule="self.all(k, k != '')"
	// +kubebuilder:validation:XValidation:message="tag contains a restricted tag matching karpenter.sh/nodepool",rule="self.all(k, k != 'karpenter.sh/nodepool')"
	// +kubebuilder:validation:XValidation:message="tag contains a restricted tag matching karpenter.sh/nodeclaim",rule="self.all(k, k !='karpenter.sh/nodeclaim')"
	// +kubebuilder:validation:XValidation:message="tag contains a restricted tag matching karpenter.k8s.cloudstack/nodeclass",rule="self.all(k, k !='karpenter.k8s.cloudstack/nodeclass')"
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// RootDiskSize specifies the size of the root disk in GB
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=1000
	// +optional
	RootDiskSize *int64 `json:"rootDiskSize,omitempty"`

	// DiskOffering specifies the disk offering for data disks
	// +optional
	DiskOffering *string `json:"diskOffering,omitempty"`

	// SSHKeyPair is the name of the SSH keypair to use for the instances
	// +optional
	SSHKeyPair *string `json:"sshKeyPair,omitempty"`
}

// ISOSelector identifies a single CloudStack ISO, by ID or by name.
type ISOSelector struct {
	// ID is the ISO id in CloudStack
	// +optional
	ID string `json:"id,omitempty"`

	// Name is the ISO name in CloudStack
	// +optional
	Name string `json:"name,omitempty"`
}

// NetworkSelectorTerm defines selection logic for a network used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type NetworkSelectorTerm struct {
	// Tags is a map of key/value tags used to select networks
	// Specifying '*' for a value selects all values for a given tag key.
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// ID is the network id in CloudStack
	// +optional
	ID string `json:"id,omitempty"`

	// Name is the network name in CloudStack
	// +optional
	Name string `json:"name,omitempty"`
}

// ServiceOfferingSelectorTerm defines selection logic for a service offering used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type ServiceOfferingSelectorTerm struct {
	// Tags is a map of key/value tags used to select service offerings
	// Specifying '*' for a value selects all values for a given tag key.
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// ID is the service offering id in CloudStack
	// +optional
	ID string `json:"id,omitempty"`

	// Name is the service offering name in CloudStack
	// +optional
	Name string `json:"name,omitempty"`
}

// TemplateSelectorTerm defines selection logic for a template used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type TemplateSelectorTerm struct {
	// Tags is a map of key/value tags used to select templates
	// Specifying '*' for a value selects all values for a given tag key.
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// ID is the template id in CloudStack
	// +optional
	ID string `json:"id,omitempty"`

	// Name is the template name in CloudStack
	// +optional
	Name string `json:"name,omitempty"`

	// OSType filters templates by operating system type
	// +optional
	OSType string `json:"osType,omitempty"`
}

// CloudStackNodeClassStatus contains the resolved state of the CloudStackNodeClass
type CloudStackNodeClassStatus struct {
	// Networks contains the resolved networks
	// +optional
	Networks []Network `json:"networks,omitempty"`

	// ServiceOfferings contains the resolved service offerings
	// +optional
	ServiceOfferings []ServiceOffering `json:"serviceOfferings,omitempty"`

	// Templates contains the resolved templates
	// +optional
	Templates []Template `json:"templates,omitempty"`

	// Conditions contains signals for health and readiness
	// +optional
	Conditions []status.Condition `json:"conditions,omitempty"`
}

// Network describes a CloudStack network
type Network struct {
	// ID is the network ID
	ID string `json:"id"`
	// Name is the network name
	Name string `json:"name"`
	// Zone is the zone where this network is available
	Zone string `json:"zone"`
	// Type is the network type (Isolated, Shared, etc.)
	Type string `json:"type,omitempty"`
}

// ServiceOffering describes a CloudStack service offering
type ServiceOffering struct {
	// ID is the service offering ID
	ID string `json:"id"`
	// Name is the service offering name
	Name string `json:"name"`
	// CPUNumber is the number of CPUs
	CPUNumber int `json:"cpuNumber"`
	// CPUSpeed is the CPU speed in MHz
	CPUSpeed int `json:"cpuSpeed,omitempty"`
	// Memory is the memory in MB
	Memory int `json:"memory"`
	// NetworkRate is the network rate in MB/s
	NetworkRate int `json:"networkRate,omitempty"`
}

// Template describes a CloudStack template
type Template struct {
	// ID is the template ID
	ID string `json:"id"`
	// Name is the template name
	Name string `json:"name"`
	// OSType is the operating system type
	OSType string `json:"osType,omitempty"`
	// Zone is the zone where this template is available
	Zone string `json:"zone"`
}

// CloudStackNodeClass is the Schema for the CloudStackNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""
// +kubebuilder:printcolumn:name="Zone",type="string",JSONPath=".spec.zone",priority=1,description=""
// +kubebuilder:resource:path=cloudstacknodeclasses,scope=Cluster,categories=karpenter,shortName={csnc,csncs}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
type CloudStackNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudStackNodeClassSpec   `json:"spec,omitempty"`
	Status CloudStackNodeClassStatus `json:"status,omitempty"`
}

// CloudStackNodeClassList contains a list of CloudStackNodeClass
// +kubebuilder:object:root=true
type CloudStackNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudStackNodeClass `json:"items"`
}

// We need to bump the CloudStackNodeClassHashVersion when we make an update to the CloudStackNodeClass CRD
const CloudStackNodeClassHashVersion = "v1"

// Hash returns a hash of the CloudStackNodeClass spec
func (in *CloudStackNodeClass) Hash() string {
	return fmt.Sprint(lo.Must(hashstructure.Hash(in.Spec, hashstructure.FormatV2, &hashstructure.HashOptions{
		SlicesAsSets:    true,
		IgnoreZeroValue: true,
		ZeroNil:         true,
	})))
}

// StatusConditions returns a ConditionSet for evaluating the status of CloudStackNodeClass
func (in *CloudStackNodeClass) StatusConditions() status.ConditionSet {
	conditionTypes := []string{
		"Ready",
	}
	return status.NewReadyConditions(conditionTypes...).For(in)
}

// GetConditions returns the status conditions (required by status.Object interface)
func (in *CloudStackNodeClass) GetConditions() []status.Condition {
	return in.Status.Conditions
}

// SetConditions sets the status conditions (required by status.Object interface)
func (in *CloudStackNodeClass) SetConditions(conditions []status.Condition) {
	in.Status.Conditions = conditions
}

// GetCondition returns the condition with the given type
func (in *CloudStackNodeClass) GetCondition(conditionType string) *status.Condition {
	for i := range in.Status.Conditions {
		if in.Status.Conditions[i].Type == conditionType {
			return &in.Status.Conditions[i]
		}
	}
	return nil
}

// SetCondition sets or updates a condition
func (in *CloudStackNodeClass) SetCondition(condition status.Condition) {
	for i := range in.Status.Conditions {
		if in.Status.Conditions[i].Type == condition.Type {
			in.Status.Conditions[i] = condition
			return
		}
	}
	in.Status.Conditions = append(in.Status.Conditions, condition)
}
