// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconciler

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestResourceFetchEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resource := NewResource(
		types.NamespacedName{Name: "pool", Namespace: "ns"},
		cl,
		func() *v1alpha1.GPUPool { return &v1alpha1.GPUPool{} },
		func(obj *v1alpha1.GPUPool) v1alpha1.GPUPoolStatus { return obj.Status },
	)

	if err := resource.Fetch(context.Background()); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if !resource.IsEmpty() {
		t.Fatalf("expected empty resource")
	}
}

func TestResourceUpdateStatusAndMetadata(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pool",
			Namespace:   "ns",
			Labels:      map[string]string{"tier": "gold"},
			Annotations: map[string]string{"note": "old"},
			Finalizers:  []string{"finalizer.gpu.deckhouse.io"},
		},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool.DeepCopy()).
		Build()

	resource := NewResource(
		types.NamespacedName{Name: "pool", Namespace: "ns"},
		cl,
		func() *v1alpha1.GPUPool { return &v1alpha1.GPUPool{} },
		func(obj *v1alpha1.GPUPool) v1alpha1.GPUPoolStatus { return obj.Status },
	)

	if err := resource.Fetch(context.Background()); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if resource.IsEmpty() {
		t.Fatalf("expected resource to exist")
	}

	changed := resource.Changed()
	changed.Status.Capacity.Total = 5
	changed.Labels["tier"] = "platinum"
	changed.Annotations["note"] = "new"
	changed.Finalizers = append(changed.Finalizers, "finalizer.gpu.deckhouse.io/secondary")

	if err := resource.Update(context.Background()); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	stored := &v1alpha1.GPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "pool", Namespace: "ns"}, stored); err != nil {
		t.Fatalf("get updated pool: %v", err)
	}
	if stored.Status.Capacity.Total != 5 {
		t.Fatalf("expected capacity updated to 5, got %d", stored.Status.Capacity.Total)
	}
	if stored.Labels["tier"] != "platinum" {
		t.Fatalf("expected label updated, got %q", stored.Labels["tier"])
	}
	if stored.Annotations["note"] != "new" {
		t.Fatalf("expected annotation updated, got %q", stored.Annotations["note"])
	}
	if len(stored.Finalizers) != 2 {
		t.Fatalf("expected finalizers updated, got %v", stored.Finalizers)
	}
}

func TestResourceUpdateMetadataOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pool",
			Namespace:   "ns",
			Labels:      map[string]string{"tier": "gold"},
			Annotations: map[string]string{"note": "old"},
		},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool.DeepCopy()).
		Build()

	resource := NewResource(
		types.NamespacedName{Name: "pool", Namespace: "ns"},
		cl,
		func() *v1alpha1.GPUPool { return &v1alpha1.GPUPool{} },
		func(obj *v1alpha1.GPUPool) v1alpha1.GPUPoolStatus { return obj.Status },
	)

	if err := resource.Fetch(context.Background()); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	changed := resource.Changed()
	changed.Annotations["note"] = "updated"

	if err := resource.Update(context.Background()); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	stored := &v1alpha1.GPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "pool", Namespace: "ns"}, stored); err != nil {
		t.Fatalf("get updated pool: %v", err)
	}
	if stored.Annotations["note"] != "updated" {
		t.Fatalf("expected annotation updated, got %q", stored.Annotations["note"])
	}
}
