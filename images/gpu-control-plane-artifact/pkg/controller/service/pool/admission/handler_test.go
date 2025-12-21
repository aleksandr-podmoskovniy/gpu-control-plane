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

package admission

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestSyncPoolAppliesDefaultsAndValidatesProvider(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))

	t.Run("empty-name", func(t *testing.T) {
		_, err := h.SyncPool(context.Background(), &v1alpha1.GPUPool{})
		if err == nil {
			t.Fatalf("expected error for empty pool name")
		}
	})

	t.Run("invalid-provider", func(t *testing.T) {
		pool := &v1alpha1.GPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool"},
			Spec: v1alpha1.GPUPoolSpec{
				Provider: "AMD",
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
			},
		}
		_, err := h.SyncPool(context.Background(), pool)
		if err == nil {
			t.Fatalf("expected unsupported provider error")
		}
	})

	t.Run("success", func(t *testing.T) {
		pool := &v1alpha1.GPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool"},
			Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			},
		}
		if _, err := h.SyncPool(context.Background(), pool); err != nil {
			t.Fatalf("SyncPool: %v", err)
		}
		if pool.Spec.Provider != defaultProvider || pool.Spec.Backend != defaultBackend {
			t.Fatalf("expected defaults applied, got %+v", pool.Spec)
		}
	})
}
