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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestSyncPoolAppliesDefaults(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
		},
	}
	if _, err := h.SyncPool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Spec.Resource.SlicesPerUnit != 1 {
		t.Fatalf("expected default slicesPerUnit=1, got %d", pool.Spec.Resource.SlicesPerUnit)
	}
	if pool.Spec.Scheduling.Strategy != v1alpha1.GPUPoolSchedulingSpread {
		t.Fatalf("expected default strategy Spread, got %s", pool.Spec.Scheduling.Strategy)
	}
	if pool.Spec.Scheduling.TopologyKey != "topology.kubernetes.io/zone" {
		t.Fatalf("expected default topology key, got %s", pool.Spec.Scheduling.TopologyKey)
	}
	if pool.Spec.Scheduling.TaintsEnabled == nil || !*pool.Spec.Scheduling.TaintsEnabled {
		t.Fatalf("expected taintsEnabled default true")
	}
}

func TestValidateSelectorsInvalidPCI(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	spec := &v1alpha1.GPUPoolSpec{
		DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{PCIVendors: []string{"zzzz"}},
		},
	}
	if err := h.validateSelectors(spec); err == nil {
		t.Fatalf("expected error for invalid pci vendor")
	}

	spec = &v1alpha1.GPUPoolSpec{
		DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Exclude: v1alpha1.GPUPoolSelectorRules{PCIDevices: []string{"123"}},
		},
	}
	if err := h.validateSelectors(spec); err == nil {
		t.Fatalf("expected error for invalid pci device length")
	}
}

func TestValidateSchedulingInvalidStrategy(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	spec := &v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: "Other"}}
	if err := h.validateScheduling(spec); err == nil {
		t.Fatalf("expected error for unsupported strategy")
	}

	spec = &v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingSpread}}
	if err := h.validateScheduling(spec); err == nil {
		t.Fatalf("expected error when topologyKey empty for Spread")
	}
}

func TestSyncPoolFailsOnInvalidProvider(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "Other",
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
		},
	}
	if _, err := h.SyncPool(context.Background(), pool); err == nil {
		t.Fatalf("expected error for unsupported provider")
	}
}

func TestSyncPoolFailsOnEmptyName(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	pool := &v1alpha1.GPUPool{
		Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	if _, err := h.SyncPool(context.Background(), pool); err == nil {
		t.Fatalf("expected error for empty metadata.name")
	}
}

func TestValidateSelectorsInvalidTaintKey(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	spec := &v1alpha1.GPUPoolSpec{
		Scheduling: v1alpha1.GPUPoolSchedulingSpec{
			Strategy: v1alpha1.GPUPoolSchedulingBinPack,
			Taints:   []v1alpha1.GPUPoolTaintSpec{{Key: ""}},
		},
	}
	if err := h.validateScheduling(spec); err == nil {
		t.Fatalf("expected error for empty taint key")
	}
}

func TestValidateSelectorsInvalidSelectors(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	spec := &v1alpha1.GPUPoolSpec{
		NodeSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "key", Operator: "BadOp"},
		}},
	}
	if err := h.validateSelectors(spec); err == nil {
		t.Fatalf("expected error for invalid nodeSelector")
	}

	spec = &v1alpha1.GPUPoolSpec{
		DeviceAssignment: v1alpha1.GPUPoolAssignmentSpec{
			AutoApproveSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "x", Operator: "BadOp"},
			}},
		},
	}
	if err := h.validateSelectors(spec); err == nil {
		t.Fatalf("expected error for invalid autoApproveSelector")
	}

	// invalid MIG profile format in selector
	spec = &v1alpha1.GPUPoolSpec{
		DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{MIGProfiles: []string{"bad"}},
		},
	}
	if err := h.validateSelectors(spec); err == nil {
		t.Fatalf("expected error for invalid MIG profile in selector")
	}
}

func TestValidateResourceErrors(t *testing.T) {
	h := NewPoolValidationHandler(testr.New(t))
	spec := &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", MIGProfile: "1g.10gb"},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for migProfile with unit=Card")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 65},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for slicesPerUnit >64")
	}
	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 0},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for slicesPerUnit <1")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit: "MIG",
		},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for backend DRA with unit!=Card")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "Card",
			SlicesPerUnit: 2,
		},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for backend DRA with slicesPerUnit>1")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "Card",
			SlicesPerUnit: 1,
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected backend DRA with Card/slicesPerUnit=1 to be valid, got %v", err)
	}

	spec = &v1alpha1.GPUPoolSpec{
		Backend: "DRA",
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:       "MIG",
			MIGProfile: "1g.10gb",
			SlicesPerUnit: 1,
		},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for backend DRA with unit=MIG")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error when MIG profile not specified")
	}

	spec = &v1alpha1.GPUPoolSpec{}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error when unit is empty")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Other"},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for unsupported unit")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "bad"},
	}
	if err := h.validateResource(spec); err == nil {
		t.Fatalf("expected error for invalid MIG profile format")
	}

	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 2},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected valid Card config, got %v", err)
	}

	spec = &v1alpha1.GPUPoolSpec{
		Resource: v1alpha1.GPUPoolResourceSpec{
			Unit:          "MIG",
			MIGProfile:    "1g.10gb",
			SlicesPerUnit: 2,
		},
	}
	if err := h.validateResource(spec); err != nil {
		t.Fatalf("expected valid MIG spec, got %v", err)
	}
}
