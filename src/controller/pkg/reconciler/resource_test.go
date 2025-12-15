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
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestResourcePatchStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", ResourceVersion: "1"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool.DeepCopy()).
		Build()

	resource := NewResource(pool, cl)
	pool.Status.Capacity.Total = 5

	if err := resource.PatchStatus(context.Background()); err != nil {
		t.Fatalf("patch status failed: %v", err)
	}

	stored := &v1alpha1.GPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "pool"}, stored); err != nil {
		t.Fatalf("get patched pool: %v", err)
	}
	if stored.Status.Capacity.Total != 5 {
		t.Fatalf("expected capacity updated to 5, got %d", stored.Status.Capacity.Total)
	}
	// original copy must stay untouched
	if resource.Original().Status.Capacity.Total != 1 {
		t.Fatalf("expected original snapshot preserved, got %d", resource.Original().Status.Capacity.Total)
	}
}

func TestResourcePatchStatusWithoutResourceVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool.DeepCopy()).
		Build()

	resource := NewResource(pool, cl)
	pool.Status.Capacity.Total = 5

	if err := resource.PatchStatus(context.Background()); err != nil {
		t.Fatalf("patch status failed: %v", err)
	}
}

func TestResourcePatchStatusErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	t.Run("nil client", func(t *testing.T) {
		resource := NewResource(pool, nil)
		if err := resource.PatchStatus(context.Background()); !errors.Is(err, errNilClient) {
			t.Fatalf("expected errNilClient, got %v", err)
		}
	})

	t.Run("nil resource current/original", func(t *testing.T) {
		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1alpha1.GPUPool{}).
			Build()

		var empty *v1alpha1.GPUPool
		resource := NewResource(empty, cl)
		if err := resource.PatchStatus(context.Background()); !errors.Is(err, errNilResource) {
			t.Fatalf("expected errNilResource, got %v", err)
		}
	})
}

func TestIsNilObjectHelper(t *testing.T) {
	t.Run("invalid interface", func(t *testing.T) {
		var v any
		if !isNilObject(v) {
			t.Fatalf("expected nil for untyped nil interface")
		}
	})

	t.Run("typed nil pointer", func(t *testing.T) {
		var pool *v1alpha1.GPUPool
		if !isNilObject(pool) {
			t.Fatalf("expected nil for typed nil pointer")
		}
	})

	t.Run("non-nil value", func(t *testing.T) {
		if isNilObject(123) {
			t.Fatalf("expected non-nil for int value")
		}
	})
}
