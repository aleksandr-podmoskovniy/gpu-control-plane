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

package admission_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/admission"
)

func TestPoolValidationDefaults(t *testing.T) {
	h := admission.NewPoolValidationHandler(testr.New(t))

	pool := &v1alpha1.GPUPool{
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Name: "resources.gpu.deckhouse.io/pool", Unit: "device"},
			Allocation: v1alpha1.GPUPoolAllocationSpec{
				Mode: v1alpha1.GPUPoolAllocationCard,
			},
		},
	}

	if _, err := h.SyncPool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Spec.Provider != "nvidia" {
		t.Fatalf("expected default provider nvidia, got %q", pool.Spec.Provider)
	}
	if pool.Spec.Backend != "device-plugin" {
		t.Fatalf("expected default backend device-plugin, got %q", pool.Spec.Backend)
	}
	if pool.Spec.Allocation.SlicesPerUnit != 1 {
		t.Fatalf("expected default slicesPerUnit=1, got %d", pool.Spec.Allocation.SlicesPerUnit)
	}
}

func TestPoolValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		pool v1alpha1.GPUPool
	}{
		{
			name: "unsupported provider",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Provider: "amd",
				Resource: v1alpha1.GPUPoolResourceSpec{Name: "r", Unit: "u"},
				Allocation: v1alpha1.GPUPoolAllocationSpec{
					Mode: v1alpha1.GPUPoolAllocationCard,
				},
			}},
		},
		{
			name: "missing mode",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource:   v1alpha1.GPUPoolResourceSpec{Name: "r", Unit: "u"},
				Allocation: v1alpha1.GPUPoolAllocationSpec{},
			}},
		},
		{
			name: "mig requires profile",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Name: "r", Unit: "u"},
				Allocation: v1alpha1.GPUPoolAllocationSpec{
					Mode: v1alpha1.GPUPoolAllocationMIG,
				},
			}},
		},
		{
			name: "card cannot set migProfile",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Name: "r", Unit: "u"},
				Allocation: v1alpha1.GPUPoolAllocationSpec{
					Mode:       v1alpha1.GPUPoolAllocationCard,
					MIGProfile: "1g.10gb",
				},
			}},
		},
		{
			name: "dra forbids slicesPerUnit>1",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Backend:  "dra",
				Resource: v1alpha1.GPUPoolResourceSpec{Name: "r", Unit: "u"},
				Allocation: v1alpha1.GPUPoolAllocationSpec{
					Mode:          v1alpha1.GPUPoolAllocationCard,
					SlicesPerUnit: 2,
				},
			}},
		},
		{
			name: "dra forbids mig mode",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Backend: "dra",
				Resource: v1alpha1.GPUPoolResourceSpec{
					Name: "r", Unit: "u",
				},
				Allocation: v1alpha1.GPUPoolAllocationSpec{
					Mode:       v1alpha1.GPUPoolAllocationMIG,
					MIGProfile: "1g.10gb",
				},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := admission.NewPoolValidationHandler(testr.New(t))
			if _, err := h.SyncPool(context.Background(), &tt.pool); err == nil {
				t.Fatalf("expected error for case %q", tt.name)
			}
		})
	}
}
