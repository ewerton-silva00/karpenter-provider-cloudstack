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

package nodeclass

import (
	"context"
	"fmt"
	"time"

	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/karpenter/pkg/events"

	v1 "github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/apis/v1"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/network"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/template"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/zone"
)

const (
	controllerName = "nodeclass"
)

// Controller is the NodeClass controller
type Controller struct {
	kubeClient       client.Client
	recorder         events.Recorder
	zoneProvider     zone.Provider
	networkProvider  network.Provider
	templateProvider template.Provider
}

// NewController creates a new NodeClass controller
func NewController(
	kubeClient client.Client,
	recorder events.Recorder,
	zoneProvider zone.Provider,
	networkProvider network.Provider,
	templateProvider template.Provider,
) *Controller {
	return &Controller{
		kubeClient:       kubeClient,
		recorder:         recorder,
		zoneProvider:     zoneProvider,
		networkProvider:  networkProvider,
		templateProvider: templateProvider,
	}
}

// Reconcile reconciles a NodeClass
func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("nodeclass", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the NodeClass
	nodeClass := &v1.CloudStackNodeClass{}
	if err := c.kubeClient.Get(ctx, req.NamespacedName, nodeClass); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Skip if being deleted
	if !nodeClass.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	// Validate zone
	if _, err := c.zoneProvider.GetByName(ctx, nodeClass.Spec.Zone); err != nil {
		c.setCondition(nodeClass, status.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "ZoneValidationFailed",
			Message: fmt.Sprintf("Zone validation failed: %v", err),
		})
		_ = c.kubeClient.Status().Update(ctx, nodeClass)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// Resolve networks
	networks, err := c.networkProvider.ResolveNetworks(ctx, nodeClass.Spec.NetworkSelectorTerms, nodeClass.Spec.Zone)
	if err != nil {
		c.setCondition(nodeClass, status.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "NetworkResolutionFailed",
			Message: fmt.Sprintf("Network resolution failed: %v", err),
		})
		_ = c.kubeClient.Status().Update(ctx, nodeClass)
		return reconcile.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Resolve templates
	templates, err := c.templateProvider.ResolveTemplates(ctx, nodeClass.Spec.TemplateSelectorTerms, nodeClass.Spec.Zone)
	if err != nil {
		c.setCondition(nodeClass, status.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplateResolutionFailed",
			Message: fmt.Sprintf("Template resolution failed: %v", err),
		})
		_ = c.kubeClient.Status().Update(ctx, nodeClass)
		return reconcile.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Update status
	nodeClass.Status.Networks = lo.Map(networks, func(n *network.Network, _ int) v1.Network {
		return v1.Network{
			ID:   n.ID,
			Name: n.Name,
			Zone: n.Zone,
			Type: n.Type,
		}
	})

	nodeClass.Status.Templates = lo.Map(templates, func(t *template.Template, _ int) v1.Template {
		return v1.Template{
			ID:     t.ID,
			Name:   t.Name,
			OSType: t.OSTypeName,
			Zone:   t.Zone,
		}
	})

	// Set Ready condition
	c.setCondition(nodeClass, status.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Ready",
		Message: "NodeClass is ready",
	})

	// Update status
	if err := c.kubeClient.Status().Update(ctx, nodeClass); err != nil {
		return reconcile.Result{}, err
	}

	logger.Info("Reconciled NodeClass successfully",
		"networks", len(networks),
		"templates", len(templates))

	// Requeue after some time to refresh cache
	return reconcile.Result{RequeueAfter: 15 * time.Minute}, nil
}

// setCondition sets a condition on the NodeClass
func (c *Controller) setCondition(nodeClass *v1.CloudStackNodeClass, condition status.Condition) {
	condition.LastTransitionTime = metav1.Now()
	condition.ObservedGeneration = nodeClass.Generation

	nodeClass.SetCondition(condition)
}

// Register registers the controller with the manager
func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named(controllerName).
		For(&v1.CloudStackNodeClass{}).
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(c)
}
