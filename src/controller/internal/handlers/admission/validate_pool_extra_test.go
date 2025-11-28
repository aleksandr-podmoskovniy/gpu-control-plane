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

func TestValidateResourceAdditionalBranches(t *testing.T) {
	h := &PoolValidationHandler{}

	if err := h.validateProvider("Unknown"); err == nil {
		t.Fatalf("expected provider validation to fail")
	}
	if err := h.validateProvider(defaultProvider); err != nil {
		t.Fatalf("expected default provider to pass: %v", err)
	}

	// unsupported unit
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Other"}}); err == nil {
		t.Fatalf("expected error for unsupported unit")
	}

	// slices per unit bounds
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 0}}); err == nil {
		t.Fatalf("expected error for slicesPerUnit <1")
	}
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 65}}); err == nil {
		t.Fatalf("expected error for slicesPerUnit >64")
	}

	// time slicing resource invalid
	spec := &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:                 "Card",
			SlicesPerUnit:        1,
			TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{{Name: "custom", SlicesPerUnit: 0}},
		},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for invalid time slicing resource")
	}

	// DRA constraints
	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "MIG",
			SlicesPerUnit: 1,
		},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for DRA with MIG unit")
	}
	spec.Resource.Unit = "Card"
	spec.Resource.SlicesPerUnit = 2
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for DRA with slicesPerUnit>1")
	}

	// DRA valid case
	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "Card",
			SlicesPerUnit: 1,
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected valid DRA card resource, got %v", err)
	}

	// invalid MIG profile format
	spec = &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "bad", SlicesPerUnit: 1}}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for invalid mig profile format")
	}

	// card with mig fields should fail
	spec = &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1, MIGProfile: "1g.10gb"}}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error when migProfile set for card unit")
	}
	spec = &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{
		Unit:          "Card",
		SlicesPerUnit: 1,
		MIGLayout:     []v1alpha1.GPUPoolMIGDeviceLayout{{Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}}}},
	}}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error when migLayout set for card unit")
	}

	// valid MIG layout with profiles and consistent slices
	count := int32(1)
	spec = &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{
		Unit:          "MIG",
		SlicesPerUnit: 2,
		MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
			SlicesPerUnit: &count,
			Profiles:      []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb", Count: &count}},
		}},
	}}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected valid mig layout, got %v", err)
	}

	// MIG layout only should still validate
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "MIG",
			SlicesPerUnit: 1,
			MIGLayout:     []v1alpha1.GPUPoolMIGDeviceLayout{{Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb"}}}},
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected mig layout without profile to be valid, got %v", err)
	}

	// MIG layout invalid should bubble error
	zero := int32(0)
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "MIG",
			SlicesPerUnit: 1,
			MIGLayout:     []v1alpha1.GPUPoolMIGDeviceLayout{{Profiles: []v1alpha1.GPUPoolMIGProfile{{Name: "1g.10gb", Count: &zero}}}},
		},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected invalid mig layout to fail via validateResource")
	}

	// Scheduling empty strategy allowed
	if err := h.validateScheduling(&v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{}}); err != nil {
		t.Fatalf("expected empty scheduling strategy to be allowed")
	}

	// Ensure empty unit rejected
	if err := h.validateResource(&v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: ""}}); err == nil {
		t.Fatalf("expected error when unit is empty")
	}
}
