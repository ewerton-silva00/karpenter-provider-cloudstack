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

// Package providerid sets the provider ID of nodes launched by Karpenter.
//
// Karpenter matches a Node to its NodeClaim by provider ID. On CloudStack
// Kubernetes Service the kubelet does not run with an external cloud provider,
// so nothing sets it. The name of the VM is derived from the NodeClaim and
// becomes the name of the Node, which is enough to find the NodeClaim.
package providerid

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/cloudprovider"
	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/instance"
)

const controllerName = "node.providerid"

// Controller sets spec.providerID on nodes launched by Karpenter
type Controller struct {
	kubeClient       client.Client
	instanceProvider instance.Provider
}

// NewController creates a new provider ID controller
func NewController(kubeClient client.Client, instanceProvider instance.Provider) *Controller {
	return &Controller{
		kubeClient:       kubeClient,
		instanceProvider: instanceProvider,
	}
}

// Reconcile sets the provider ID of a node from the NodeClaim it was launched for
func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	node := &corev1.Node{}
	if err := c.kubeClient.Get(ctx, req.NamespacedName, node); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if node.Spec.ProviderID != "" {
		return reconcile.Result{}, nil
	}

	nodeClaimName, ok := instance.NodeClaimNameFromVMName(node.Name)
	if !ok {
		return reconcile.Result{}, nil
	}
	nodeClaim := &karpv1.NodeClaim{}
	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClaimName}, nodeClaim); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if nodeClaim.Status.ProviderID == "" {
		// The instance was launched but the NodeClaim status is not updated yet
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	stored := node.DeepCopy()
	node.Spec.ProviderID = nodeClaim.Status.ProviderID
	if err := c.kubeClient.Patch(ctx, node, client.MergeFrom(stored)); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	log.FromContext(ctx).Info("Set provider ID", "node", node.Name, "providerID", node.Spec.ProviderID)

	// The node has joined, so the binaries on the ISO are installed. Detaching is
	// best effort: a leftover ISO does not affect the node.
	if id, err := cloudprovider.ParseProviderID(node.Spec.ProviderID); err == nil {
		if err := c.instanceProvider.DetachISO(ctx, id); err != nil {
			log.FromContext(ctx).Error(err, "Failed to detach ISO", "node", node.Name)
		}
	}

	return reconcile.Result{}, nil
}

// Register registers the controller with the manager
func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named(controllerName).
		For(&corev1.Node{}).
		Complete(c)
}
