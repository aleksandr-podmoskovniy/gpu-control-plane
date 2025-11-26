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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/admission"
)

func TestPoolValidationDefaults(t *testing.T) {
	h := admission.NewPoolValidationHandler(testr.New(t))

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool-default",
		},
		Spec: v1alpha1.GPUPoolSpec{
			Resource:   v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{},
			Access: v1alpha1.GPUPoolAccessSpec{
				Namespaces:      []string{" ns1 ", "ns1"},
				ServiceAccounts: []string{"sa", "sa"},
			},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{
					PCIVendors: []string{"10de", "10de"},
				},
			},
		},
	}

	if _, err := h.SyncPool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Spec.Provider != "Nvidia" {
		t.Fatalf("expected default provider Nvidia, got %q", pool.Spec.Provider)
	}
	if pool.Spec.Backend != "DevicePlugin" {
		t.Fatalf("expected default backend DevicePlugin, got %q", pool.Spec.Backend)
	}
	if pool.Spec.Resource.SlicesPerUnit != 1 {
		t.Fatalf("expected default slicesPerUnit=1, got %d", pool.Spec.Resource.SlicesPerUnit)
	}
	if pool.Spec.Scheduling.Strategy != v1alpha1.GPUPoolSchedulingSpread {
		t.Fatalf("expected default strategy Spread, got %s", pool.Spec.Scheduling.Strategy)
	}
	if pool.Spec.Scheduling.TopologyKey == "" {
		t.Fatalf("expected topologyKey default when strategy=Spread")
	}
	if len(pool.Spec.Access.Namespaces) != 1 || len(pool.Spec.Access.ServiceAccounts) != 1 {
		t.Fatalf("expected access lists deduped, got %+v", pool.Spec.Access)
	}
	if pool.Spec.DeviceSelector == nil || len(pool.Spec.DeviceSelector.Include.PCIVendors) != 1 {
		t.Fatalf("expected deduped pci vendors")
	}
}

func TestPoolValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		pool v1alpha1.GPUPool
	}{
		{
			name: "missing resource unit",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{},
			}},
		},
		{
			name: "invalid mig profile format",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "weird"},
			}},
		},
		{
			name: "invalid pci vendor",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
					Include: v1alpha1.GPUPoolSelectorRules{PCIVendors: []string{"123"}},
				},
			}},
		},
		{
			name: "invalid pci device",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
					Include: v1alpha1.GPUPoolSelectorRules{PCIDevices: []string{"gggg"}},
				},
			}},
		},
		{
			name: "invalid taint key",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				Scheduling: v1alpha1.GPUPoolSchedulingSpec{
					Taints: []v1alpha1.GPUPoolTaintSpec{{Key: "   "}},
				},
			}},
		},
		{
			name: "invalid node selector",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"bad key": "v"},
				},
			}},
		},
		{
			name: "invalid auto approve selector",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				DeviceAssignment: v1alpha1.GPUPoolAssignmentSpec{
					AutoApproveSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"": "v"},
					},
				},
			}},
		},
		{
			name: "invalid scheduling strategy",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				Scheduling: v1alpha1.GPUPoolSchedulingSpec{
					Strategy: "Unknown",
				},
			}},
		},
		{
			name: "invalid exclude mig profile",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
				DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
					Exclude: v1alpha1.GPUPoolSelectorRules{MIGProfiles: []string{"bad"}},
				},
			}},
		},
		{
			name: "unsupported provider",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Provider: "AMD",
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			}},
		},
		{
			name: "slices per unit too low",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: -1},
			}},
		},
		{
			name: "slices per unit too high",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 100},
			}},
		},
		{
			name: "invalid time slicing",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{
					Unit:                 "Card",
					SlicesPerUnit:        1,
					TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{{Name: "gpu.deckhouse.io/p", SlicesPerUnit: 0}},
				},
			}},
		},
		{
			name: "dra with mig",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Backend:  "DRA",
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"},
			}},
		},
		{
			name: "dra with slices",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Backend:  "DRA",
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 2},
			}},
		},
		{
			name: "empty name",
			pool: v1alpha1.GPUPool{},
		},
		{
			name: "unsupported unit",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Unknown"},
			}},
		},
		{
			name: "card with mig profile set",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", MIGProfile: "1g.10gb"},
			}},
		},
		{
			name: "mig without profile or layout",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"},
			}},
		},
		{
			name: "card with mig layout",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Resource: v1alpha1.GPUPoolResourceSpec{
					Unit:      "Card",
					MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}}}},
				},
			}},
		},
		{
			name: "dra with mig layout",
			pool: v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{
				Backend: "DRA",
				Resource: v1alpha1.GPUPoolResourceSpec{
					Unit:      "Card",
					MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}}}},
				},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := admission.NewPoolValidationHandler(testr.New(t))
			if tt.pool.Name == "" && tt.name != "empty name" {
				tt.pool.Name = "pool"
			}
			if _, err := h.SyncPool(context.Background(), &tt.pool); err == nil {
				t.Fatalf("expected error for case %q", tt.name)
			}
		})
	}
}

func TestPoolValidationDedupesSelectors(t *testing.T) {
	h := admission.NewPoolValidationHandler(testr.New(t))
	pool := v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool",
		},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{
					InventoryIDs: []string{"a", "a", " b "},
					MIGProfiles:  []string{"1g.10gb", "1g.10gb"},
					PCIVendors:   []string{"10de", "10de"},
				},
			},
		}}

	if _, err := h.SyncPool(context.Background(), &pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pool.Spec.DeviceSelector.Include.InventoryIDs) != 2 {
		t.Fatalf("expected deduped inventoryIDs, got %v", pool.Spec.DeviceSelector.Include.InventoryIDs)
	}
	if len(pool.Spec.DeviceSelector.Include.PCIVendors) != 1 {
		t.Fatalf("expected deduped pci vendors, got %v", pool.Spec.DeviceSelector.Include.PCIVendors)
	}
}
