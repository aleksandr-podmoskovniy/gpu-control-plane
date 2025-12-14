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

package gpupool

import (
	"reflect"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestFilterDevicesIncludeExclude(t *testing.T) {
	devs := []v1alpha1.GPUDevice{
		{
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "id-a",
				Hardware: v1alpha1.GPUDeviceHardware{
					Product: "A100",
					PCI:     v1alpha1.PCIAddress{Vendor: "10de", Device: "20b0"},
					MIG:     v1alpha1.GPUMIGConfig{Capable: true, ProfilesSupported: []string{"1g.10gb"}},
				},
			},
		},
		{
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "id-b",
				Hardware: v1alpha1.GPUDeviceHardware{
					Product: "V100",
					PCI:     v1alpha1.PCIAddress{Vendor: "10de", Device: "1db4"},
					MIG:     v1alpha1.GPUMIGConfig{Capable: false},
				},
			},
		},
	}

	sel := &v1alpha1.GPUPoolDeviceSelector{
		Include: v1alpha1.GPUPoolSelectorRules{
			Products:    []string{"A100"},
			PCIVendors:  []string{"10de"},
			MIGProfiles: []string{"1g.10gb"},
		},
	}
	got := FilterDevices(devs, sel)
	if len(got) != 1 || got[0].Status.InventoryID != "id-a" {
		t.Fatalf("expected only id-a, got %+v", got)
	}

	// Exclude matching vendor should drop both.
	sel.Exclude = v1alpha1.GPUPoolSelectorRules{PCIVendors: []string{"10de"}}
	if filtered := FilterDevices(devs, sel); len(filtered) != 0 {
		t.Fatalf("expected empty after exclude vendor, got %v", filtered)
	}

	// Exclude MIG capable only removes first.
	sel.Include = v1alpha1.GPUPoolSelectorRules{}
	sel.Exclude = v1alpha1.GPUPoolSelectorRules{MIGCapable: boolPtr(true)}
	if filtered := FilterDevices(devs, sel); len(filtered) != 1 || filtered[0].Status.InventoryID != "id-b" {
		t.Fatalf("expected only id-b after exclude MIG capable, got %v", filtered)
	}
}

func TestMatchesIncludeEmptySelector(t *testing.T) {
	dev := v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{InventoryID: "id-a"}}
	if !matchesInclude(v1alpha1.GPUPoolSelectorRules{}, dev) {
		t.Fatalf("empty include should match everything")
	}
}

func TestAnyMIGProfile(t *testing.T) {
	if anyMIGProfile([]string{"1g.5gb"}, []string{"1g.10gb"}) {
		t.Fatalf("expected false when no profiles match")
	}
	if !anyMIGProfile([]string{"1g.5gb", "1g.10gb"}, []string{"1g.10gb"}) {
		t.Fatalf("expected true when at least one matches")
	}
}

func TestContainsHelper(t *testing.T) {
	if !contains([]string{"a", "b"}, "a") {
		t.Fatalf("expected contains to find value")
	}
	if contains([]string{"a", "b"}, "c") {
		t.Fatalf("expected contains to return false")
	}
}

func boolPtr(v bool) *bool { return &v }

func TestFilterDevicesNilSelectorCopiesSlice(t *testing.T) {
	devs := []v1alpha1.GPUDevice{{Status: v1alpha1.GPUDeviceStatus{InventoryID: "x"}}}
	res := FilterDevices(devs, nil)
	if !reflect.DeepEqual(devs, res) {
		t.Fatalf("expected copy of devices when selector is nil")
	}
	if &devs[0] == &res[0] {
		t.Fatalf("expected distinct underlying elements")
	}
}
