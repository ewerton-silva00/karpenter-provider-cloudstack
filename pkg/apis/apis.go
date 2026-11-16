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

package apis

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/apis/v1"
)

const (
	Group = "karpenter.k8s.cloudstack"
)

var (
	// SchemeBuilder builds a scheme with all the API types
	SchemeBuilder = runtime.NewSchemeBuilder(
		v1.SchemeBuilder.AddToScheme,
	)

	AddToScheme = SchemeBuilder.AddToScheme
)

// CRDs contains all the CRDs for the CloudStack provider
var CRDs = []runtime.Object{
	&v1.CloudStackNodeClass{},
}

// GroupVersion returns the group version for the CloudStack provider
func GroupVersion() schema.GroupVersion {
	return v1.SchemeGroupVersion
}
