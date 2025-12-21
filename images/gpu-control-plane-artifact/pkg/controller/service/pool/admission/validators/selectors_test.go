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

package validators

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestSelectorsValidator(t *testing.T) {
	validate := Selectors()

	t.Run("nil-device-selector", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{}
		if err := validate(spec); err != nil {
			t.Fatalf("expected selectors to be initialised when nil, got %v", err)
		}
		if spec.DeviceSelector == nil {
			t.Fatalf("expected deviceSelector to be set")
		}
	})

	t.Run("valid-selectors", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{
					PCIVendors:  []string{"10de"},
					PCIDevices:  []string{"20b0"},
					MIGProfiles: []string{"1g.10gb"},
				},
				Exclude: v1alpha1.GPUPoolSelectorRules{
					PCIVendors:  []string{"10de"},
					MIGProfiles: []string{"1g.10gb"},
				},
			},
		}
		if err := validate(spec); err != nil {
			t.Fatalf("expected valid selectors, got %v", err)
		}
	})

	t.Run("dedup-trims", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{
					InventoryIDs: []string{" a ", "b", "a"},
				},
			},
		}
		if err := validate(spec); err != nil {
			t.Fatalf("expected valid selectors, got %v", err)
		}
		want := []string{"a", "b"}
		if !reflect.DeepEqual(spec.DeviceSelector.Include.InventoryIDs, want) {
			t.Fatalf("expected deduped inventory IDs %v, got %v", want, spec.DeviceSelector.Include.InventoryIDs)
		}
	})

	t.Run("invalid-pci-vendor", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{PCIVendors: []string{"xyz"}},
		}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected invalid pci vendor to error")
		}
	})

	t.Run("invalid-pci-device", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{PCIDevices: []string{"20b"}},
		}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected invalid pci device to error")
		}
	})

	t.Run("invalid-mig-profile", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
			Include: v1alpha1.GPUPoolSelectorRules{MIGProfiles: []string{"bad"}},
		}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected invalid mig profile to error")
		}
	})

	t.Run("invalid-node-selector", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{NodeSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "a", Operator: "BadOp", Values: []string{"b"}}},
		}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected invalid nodeSelector to error")
		}
	})

	t.Run("invalid-auto-approve-selector", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{DeviceAssignment: v1alpha1.GPUPoolAssignmentSpec{
			AutoApproveSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "a", Operator: "BadOp", Values: []string{"b"}}},
			},
		}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected invalid deviceAssignment.autoApproveSelector to error")
		}
	})
}
