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

package pool

import (
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilterDevicesIncludeExcludeRules(t *testing.T) {
	dev := v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id1",
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "prodA",
				PCI:     v1alpha1.PCIAddress{Vendor: "10de", Device: "1a2b"},
				MIG: v1alpha1.GPUMIGConfig{
					Capable:           true,
					ProfilesSupported: []string{"1g.10gb"},
				},
			},
		},
	}
	devs := []v1alpha1.GPUDevice{dev}

	// Empty include means pass, exclude by product blocks.
	sel := &v1alpha1.GPUPoolDeviceSelector{
		Exclude: v1alpha1.GPUPoolSelectorRules{Products: []string{"prodA"}},
	}
	if got := FilterDevices(devs, sel); len(got) != 0 {
		t.Fatalf("expected device to be excluded by product, got %d", len(got))
	}

	// Include all fields must match.
	sel = &v1alpha1.GPUPoolDeviceSelector{
		Include: v1alpha1.GPUPoolSelectorRules{
			InventoryIDs: []string{"id1"},
			Products:     []string{"prodA"},
			PCIVendors:   []string{"10de"},
			PCIDevices:   []string{"1a2b"},
			MIGProfiles:  []string{"1g.10gb"},
			MIGCapable:   ptrTo(true),
		},
	}
	if got := FilterDevices(devs, sel); len(got) != 1 {
		t.Fatalf("expected device to match include rules, got %d", len(got))
	}

	// MIGCapable mismatch should filter out.
	sel.Include.MIGCapable = ptrTo(false)
	if got := FilterDevices(devs, sel); len(got) != 0 {
		t.Fatalf("expected device to be filtered by MIGCapable mismatch, got %d", len(got))
	}
}

func TestMatchesExcludeBranches(t *testing.T) {
	dev := v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id2",
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "prodB",
				PCI:     v1alpha1.PCIAddress{Vendor: "1234", Device: "5678"},
				MIG: v1alpha1.GPUMIGConfig{
					Capable:           false,
					ProfilesSupported: []string{"2g.20gb"},
				},
			},
		},
	}

	exclude := v1alpha1.GPUPoolSelectorRules{
		InventoryIDs: []string{"id2"},
	}
	if !matchesExclude(exclude, dev) {
		t.Fatalf("expected exclude by inventory id")
	}

	exclude = v1alpha1.GPUPoolSelectorRules{Products: []string{"prodB"}}
	if !matchesExclude(exclude, dev) {
		t.Fatalf("expected exclude by product")
	}

	exclude = v1alpha1.GPUPoolSelectorRules{PCIVendors: []string{"1234"}}
	if !matchesExclude(exclude, dev) {
		t.Fatalf("expected exclude by pci vendor")
	}

	exclude = v1alpha1.GPUPoolSelectorRules{PCIDevices: []string{"5678"}}
	if !matchesExclude(exclude, dev) {
		t.Fatalf("expected exclude by pci device")
	}

	exclude = v1alpha1.GPUPoolSelectorRules{MIGCapable: ptrTo(false)}
	if !matchesExclude(exclude, dev) {
		t.Fatalf("expected exclude by mig capable")
	}

	exclude = v1alpha1.GPUPoolSelectorRules{MIGProfiles: []string{"2g.20gb"}}
	if !matchesExclude(exclude, dev) {
		t.Fatalf("expected exclude by mig profile")
	}
}

func TestMatchesIncludeBranches(t *testing.T) {
	dev := v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "idx",
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "prodX",
				PCI:     v1alpha1.PCIAddress{Vendor: "abcd", Device: "ef01"},
				MIG: v1alpha1.GPUMIGConfig{
					Capable:           true,
					ProfilesSupported: []string{"4g.20gb"},
				},
			},
		},
	}

	cases := []v1alpha1.GPUPoolSelectorRules{
		{InventoryIDs: []string{"other"}}, // inventory mismatch
		{Products: []string{"other"}},     // product mismatch
		{PCIVendors: []string{"ffff"}},    // vendor mismatch
		{PCIDevices: []string{"eeee"}},    // device mismatch
		{MIGCapable: ptrTo(false)},        // mig capable mismatch
		{MIGProfiles: []string{"1g.5gb"}}, // mig profile mismatch
		{InventoryIDs: []string{"idx"}, Products: []string{"prodX"}, PCIVendors: []string{"abcd"}, PCIDevices: []string{"ef01"}, MIGCapable: ptrTo(true), MIGProfiles: []string{"4g.20gb"}}, // success
	}

	for i, inc := range cases {
		ok := matchesInclude(inc, dev)
		if i < len(cases)-1 && ok {
			t.Fatalf("case %d should fail", i)
		}
		if i == len(cases)-1 && !ok {
			t.Fatalf("last case should pass")
		}
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestMIGProfilesUsesTypesWhenProfilesSupportedEmpty(t *testing.T) {
	mig := v1alpha1.GPUMIGConfig{
		Types: []v1alpha1.GPUMIGTypeCapacity{
			{Name: " 1g.10gb ", Count: 1},
			{Name: "", Count: 2},
			{Name: "   ", Count: 3},
		},
	}
	got := migProfiles(mig)
	if len(got) != 1 || got[0] != "1g.10gb" {
		t.Fatalf("unexpected profiles: %#v", got)
	}
}

func TestMatchesInventoryIDFallsBackToDeviceName(t *testing.T) {
	dev := v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-name"},
		Status:     v1alpha1.GPUDeviceStatus{InventoryID: ""},
	}

	if !matchesInclude(v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"dev-name"}}, dev) {
		t.Fatalf("expected include to match by device name when inventoryID is empty")
	}
	if !matchesExclude(v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"dev-name"}}, dev) {
		t.Fatalf("expected exclude to match by device name when inventoryID is empty")
	}
}
