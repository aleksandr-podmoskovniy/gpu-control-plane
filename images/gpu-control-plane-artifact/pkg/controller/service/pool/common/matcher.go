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

package common

import (
	"strings"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

// FilterDevices applies include/exclude selectors to the provided device list.
func FilterDevices(devices []v1alpha1.GPUDevice, sel *v1alpha1.GPUPoolDeviceSelector) []v1alpha1.GPUDevice {
	if sel == nil {
		out := make([]v1alpha1.GPUDevice, len(devices))
		copy(out, devices)
		return out
	}

	out := make([]v1alpha1.GPUDevice, 0, len(devices))
	for _, dev := range devices {
		if matchesExclude(sel.Exclude, dev) {
			continue
		}
		if matchesInclude(sel.Include, dev) {
			out = append(out, dev)
		}
	}
	return out
}

func matchesInclude(include v1alpha1.GPUPoolSelectorRules, dev v1alpha1.GPUDevice) bool {
	if len(include.InventoryIDs) == 0 &&
		len(include.Products) == 0 &&
		len(include.PCIVendors) == 0 &&
		len(include.PCIDevices) == 0 &&
		len(include.MIGProfiles) == 0 &&
		include.MIGCapable == nil {
		return true
	}

	inventoryID := strings.TrimSpace(dev.Status.InventoryID)
	if inventoryID == "" {
		inventoryID = dev.Name
	}

	if len(include.InventoryIDs) > 0 && !contains(include.InventoryIDs, inventoryID) {
		return false
	}
	if len(include.Products) > 0 && !contains(include.Products, dev.Status.Hardware.Product) {
		return false
	}
	if len(include.PCIVendors) > 0 && !contains(include.PCIVendors, dev.Status.Hardware.PCI.Vendor) {
		return false
	}
	if len(include.PCIDevices) > 0 && !contains(include.PCIDevices, dev.Status.Hardware.PCI.Device) {
		return false
	}
	if include.MIGCapable != nil && dev.Status.Hardware.MIG.Capable != *include.MIGCapable {
		return false
	}
	if len(include.MIGProfiles) > 0 && !anyMIGProfile(include.MIGProfiles, migProfiles(dev.Status.Hardware.MIG)) {
		return false
	}
	return true
}

func matchesExclude(exclude v1alpha1.GPUPoolSelectorRules, dev v1alpha1.GPUDevice) bool {
	inventoryID := strings.TrimSpace(dev.Status.InventoryID)
	if inventoryID == "" {
		inventoryID = dev.Name
	}

	if len(exclude.InventoryIDs) > 0 && contains(exclude.InventoryIDs, inventoryID) {
		return true
	}
	if len(exclude.Products) > 0 && contains(exclude.Products, dev.Status.Hardware.Product) {
		return true
	}
	if len(exclude.PCIVendors) > 0 && contains(exclude.PCIVendors, dev.Status.Hardware.PCI.Vendor) {
		return true
	}
	if len(exclude.PCIDevices) > 0 && contains(exclude.PCIDevices, dev.Status.Hardware.PCI.Device) {
		return true
	}
	if exclude.MIGCapable != nil && dev.Status.Hardware.MIG.Capable == *exclude.MIGCapable {
		return true
	}
	if len(exclude.MIGProfiles) > 0 && anyMIGProfile(exclude.MIGProfiles, migProfiles(dev.Status.Hardware.MIG)) {
		return true
	}
	return false
}

func anyMIGProfile(want []string, supported []string) bool {
	for _, w := range want {
		if contains(supported, w) {
			return true
		}
	}
	return false
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func migProfiles(m v1alpha1.GPUMIGConfig) []string {
	if len(m.ProfilesSupported) > 0 {
		return m.ProfilesSupported
	}
	profiles := make([]string, 0, len(m.Types))
	for _, t := range m.Types {
		name := strings.TrimSpace(t.Name)
		if name != "" {
			profiles = append(profiles, name)
		}
	}
	return profiles
}
