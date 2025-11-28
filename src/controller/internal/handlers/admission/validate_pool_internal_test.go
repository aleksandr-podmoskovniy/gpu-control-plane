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
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestValidateResourceBranches(t *testing.T) {
	h := &PoolValidationHandler{}

	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: ""}}); err == nil {
		t.Fatalf("expected error for empty unit")
	}
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 2}}); err != nil {
		t.Fatalf("expected valid card resource, got %v", err)
	}
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb", SlicesPerUnit: 1}}); err != nil {
		t.Fatalf("expected valid mig resource, got %v", err)
	}
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"}}); err == nil {
		t.Fatalf("expected error for missing migProfile")
	}
}

func TestValidateSchedulingBranches(t *testing.T) {
	h := &PoolValidationHandler{}
	spec := &v1alpha1.GPUPoolSpec{
		Scheduling: v1alpha1.GPUPoolSchedulingSpec{
			Strategy: "Unknown",
		},
	}
	if err := h.validateScheduling(spec); err == nil {
		t.Fatalf("expected error for invalid strategy")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Scheduling: v1alpha1.GPUPoolSchedulingSpec{
			Strategy:    v1alpha1.GPUPoolSchedulingSpread,
			TopologyKey: "topology.kubernetes.io/zone",
			Taints: []v1alpha1.GPUPoolTaintSpec{
				{Key: "k", Value: " v ", Effect: " NoSchedule "},
			},
		},
	}
	if err := h.validateScheduling(spec); err != nil {
		t.Fatalf("expected valid scheduling, got %v", err)
	}
	if spec.Scheduling.Taints[0].Effect != "NoSchedule" {
		t.Fatalf("expected trimmed effect, got %q", spec.Scheduling.Taints[0].Effect)
	}

	// empty strategy defaults to no error
	spec = &v1alpha1.GPUPoolSpec{}
	if err := h.validateScheduling(spec); err != nil {
		t.Fatalf("expected empty scheduling to be valid, got %v", err)
	}

	// include exclude selectors loops
	selSpec := &v1alpha1.GPUPoolSpec{
		DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{
				PCIVendors: []string{"10de"},
			},
			Exclude: v1alpha1.GPUPoolSelectorRules{
				PCIDevices: []string{"20b0"},
			},
		},
	}
	if err := h.validateSelectors(selSpec); err != nil {
		t.Fatalf("expected valid selectors, got %v", err)
	}

	if err := h.validateSelectors(&v1alpha1.GPUPoolSpec{}); err != nil {
		t.Fatalf("expected selectors to be initialised when nil, got %v", err)
	}

	// cover include PCIDevices path with valid data
	selSpec = &v1alpha1.GPUPoolSpec{
		DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{
				PCIDevices: []string{"20b0"},
				MIGProfiles: []string{
					"1g.10gb",
				},
			},
		},
	}
	if err := h.validateSelectors(selSpec); err != nil {
		t.Fatalf("expected valid include selectors, got %v", err)
	}

	// BinPack strategy success path
	spec = &v1alpha1.GPUPoolSpec{
		Scheduling: v1alpha1.GPUPoolSchedulingSpec{
			Strategy: v1alpha1.GPUPoolSchedulingBinPack,
		},
	}
	if err := h.validateScheduling(spec); err != nil {
		t.Fatalf("expected binpack scheduling valid, got %v", err)
	}

	// Spread without topology must error
	spec = &v1alpha1.GPUPoolSpec{
		Scheduling: v1alpha1.GPUPoolSchedulingSpec{
			Strategy: v1alpha1.GPUPoolSchedulingSpread,
			Taints:   []v1alpha1.GPUPoolTaintSpec{},
		},
	}
	if err := h.validateScheduling(spec); err == nil {
		t.Fatalf("expected error for spread without topology")
	}

	// Exclude selectors exercise loop
	selSpec = &v1alpha1.GPUPoolSpec{
		DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Exclude: v1alpha1.GPUPoolSelectorRules{
				PCIVendors: []string{"10de"},
				MIGProfiles: []string{
					"1g.10gb",
				},
			},
		},
	}
	if err := h.validateSelectors(selSpec); err != nil {
		t.Fatalf("expected valid exclude selectors, got %v", err)
	}
}

func TestValidateMIGLayoutBranches(t *testing.T) {
	h := &PoolValidationHandler{}

	// Missing profiles when migProfile empty
	spec := &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:      "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected error for empty profiles without migProfile")
	}

	// Invalid profile format
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "bad"}},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected error for invalid mig profile format")
	}

	// Invalid count
	minusOne := int32(0)
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb", Count: &minusOne}},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected error for invalid count")
	}

	// SlicesPerUnit mismatch inside layout
	s1 := int32(2)
	s2 := int32(3)
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{
					{Name: "1g.10gb", SlicesPerUnit: &s1},
					{Name: "2g.20gb", SlicesPerUnit: &s2},
				},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected error for mismatched profile slices")
	}

	// Conflict between layout slices and resource slicesPerUnit
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "MIG",
			SlicesPerUnit: 4,
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{
					{Name: "1g.10gb", SlicesPerUnit: &s1},
				},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected conflict between resource slices and layout slices")
	}

	// Valid layout with shared slicesPerUnit
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{
					{Name: "1g.10gb", SlicesPerUnit: &s1},
					{Name: "2g.20gb", SlicesPerUnit: &s1},
				},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err != nil {
		t.Fatalf("expected valid layout, got %v", err)
	}

	// Invalid slicesPerUnit range
	badSlices := int32(0)
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{
					{Name: "1g.10gb", SlicesPerUnit: &badSlices},
				},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected error for invalid slicesPerUnit range")
	}

	// Missing profile name
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
				Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: ""}},
			}},
		},
	}
	if err := h.validateMIGLayout(spec); err == nil {
		t.Fatalf("expected error for empty profile name")
	}
}

func TestValidateResourceAdditionalPaths(t *testing.T) {
	h := &PoolValidationHandler{}

	// MIG with layout passes through validateResource and migLayout
	spec := &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
			MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{
				{Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}}},
			},
			SlicesPerUnit: 1,
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected valid MIG layout resource, got %v", err)
	}

	// Backend DRA valid path
	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "Card",
			SlicesPerUnit: 1,
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected DRA card resource to be valid, got %v", err)
	}

	// TimeSlicingResources valid path
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "Card",
			SlicesPerUnit: 1,
			TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{
				{Name: "gpu.deckhouse.io/pool", SlicesPerUnit: 2},
			},
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected valid timeSlicingResources, got %v", err)
	}
}

func TestDedupStringsHelper(t *testing.T) {
	out := dedupStrings([]string{"", " ", "a", "a", " b "})
	if len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("unexpected dedup result: %v", out)
	}
	if res := dedupStrings(nil); len(res) != 0 {
		t.Fatalf("expected empty slice for nil input, got %v", res)
	}
}
