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

package providerid

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/ewerton-silva00/karpenter-provider-cloudstack/pkg/providers/instance"
)

type fakeInstanceProvider struct {
	instance.Provider
	detached []string
}

func (f *fakeInstanceProvider) DetachISO(_ context.Context, id string) error {
	f.detached = append(f.detached, id)
	return nil
}

func node(name, providerID string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: corev1.NodeSpec{ProviderID: providerID}}
}

func nodeClaim(name, providerID string) *karpv1.NodeClaim {
	return &karpv1.NodeClaim{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: karpv1.NodeClaimStatus{ProviderID: providerID}}
}

func TestReconcile(t *testing.T) {
	const providerID = "cloudstack://RN-1a/vm-1"

	tests := []struct {
		name         string
		objects      []client.Object
		node         string
		want         string
		wantRequeue  bool
		wantDetached []string
	}{
		{
			name:         "sets the provider ID from the nodeclaim and detaches the ISO",
			objects:      []client.Object{node("karpenter-default-abc12", ""), nodeClaim("default-abc12", providerID)},
			node:         "karpenter-default-abc12",
			want:         providerID,
			wantDetached: []string{"vm-1"},
		},
		{
			name:    "leaves an existing provider ID alone",
			objects: []client.Object{node("karpenter-default-abc12", "other://id"), nodeClaim("default-abc12", providerID)},
			node:    "karpenter-default-abc12",
			want:    "other://id",
		},
		{
			name:    "ignores nodes not launched by Karpenter",
			objects: []client.Object{node("cks-apps-node-1", ""), nodeClaim("cks-apps-node-1", providerID)},
			node:    "cks-apps-node-1",
			want:    "",
		},
		{
			name:    "ignores nodes without a nodeclaim",
			objects: []client.Object{node("karpenter-default-abc12", "")},
			node:    "karpenter-default-abc12",
			want:    "",
		},
		{
			name:        "waits for the nodeclaim to be launched",
			objects:     []client.Object{node("karpenter-default-abc12", ""), nodeClaim("default-abc12", "")},
			node:        "karpenter-default-abc12",
			want:        "",
			wantRequeue: true,
		},
		{
			name: "node is gone",
			node: "karpenter-default-abc12",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			kubeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tt.objects...).WithStatusSubresource(&karpv1.NodeClaim{}).Build()
			for _, o := range tt.objects {
				if nc, ok := o.(*karpv1.NodeClaim); ok {
					if err := kubeClient.Status().Update(ctx, nc); err != nil {
						t.Fatal(err)
					}
				}
			}
			instances := &fakeInstanceProvider{}

			result, err := NewController(kubeClient, instances).Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKey{Name: tt.node}})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := result.RequeueAfter > 0; got != tt.wantRequeue {
				t.Errorf("requeue = %v, want %v", got, tt.wantRequeue)
			}
			if len(instances.detached) != len(tt.wantDetached) || (len(tt.wantDetached) > 0 && instances.detached[0] != tt.wantDetached[0]) {
				t.Errorf("detached = %v, want %v", instances.detached, tt.wantDetached)
			}

			got := &corev1.Node{}
			if err := kubeClient.Get(ctx, client.ObjectKey{Name: tt.node}, got); err != nil {
				if len(tt.objects) == 0 {
					return
				}
				t.Fatal(err)
			}
			if got.Spec.ProviderID != tt.want {
				t.Errorf("providerID = %q, want %q", got.Spec.ProviderID, tt.want)
			}
		})
	}
}
