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

package controllers

import (
	"context"

	"github.com/awslabs/operatorpkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/karpenter/pkg/events"

	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/controllers/node/providerid"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/controllers/nodeclass"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/instance"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/network"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/template"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/zone"
)

// NewControllers returns a slice of CloudStack-specific controllers
func NewControllers(
	ctx context.Context,
	mgr manager.Manager,
	kubeClient client.Client,
	recorder events.Recorder,
	zoneProvider zone.Provider,
	networkProvider network.Provider,
	templateProvider template.Provider,
	instanceProvider instance.Provider,
) []controller.Controller {
	return []controller.Controller{
		nodeclass.NewController(
			kubeClient,
			recorder,
			zoneProvider,
			networkProvider,
			templateProvider,
		),
		providerid.NewController(kubeClient, instanceProvider),
	}
}
