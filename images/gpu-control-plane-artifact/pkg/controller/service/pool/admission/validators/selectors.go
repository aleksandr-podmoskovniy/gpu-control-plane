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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func Selectors() SpecValidator {
	return func(spec *v1alpha1.GPUPoolSpec) error {
		if spec.DeviceSelector == nil {
			spec.DeviceSelector = &v1alpha1.GPUPoolDeviceSelector{}
		}

		spec.DeviceSelector.Include.InventoryIDs = dedupStrings(spec.DeviceSelector.Include.InventoryIDs)
		spec.DeviceSelector.Include.Products = dedupStrings(spec.DeviceSelector.Include.Products)
		spec.DeviceSelector.Include.PCIVendors = dedupStrings(spec.DeviceSelector.Include.PCIVendors)
		spec.DeviceSelector.Include.PCIDevices = dedupStrings(spec.DeviceSelector.Include.PCIDevices)
		spec.DeviceSelector.Include.MIGProfiles = dedupStrings(spec.DeviceSelector.Include.MIGProfiles)

		spec.DeviceSelector.Exclude.InventoryIDs = dedupStrings(spec.DeviceSelector.Exclude.InventoryIDs)
		spec.DeviceSelector.Exclude.Products = dedupStrings(spec.DeviceSelector.Exclude.Products)
		spec.DeviceSelector.Exclude.PCIVendors = dedupStrings(spec.DeviceSelector.Exclude.PCIVendors)
		spec.DeviceSelector.Exclude.PCIDevices = dedupStrings(spec.DeviceSelector.Exclude.PCIDevices)
		spec.DeviceSelector.Exclude.MIGProfiles = dedupStrings(spec.DeviceSelector.Exclude.MIGProfiles)

		for _, vendor := range append(spec.DeviceSelector.Include.PCIVendors, spec.DeviceSelector.Exclude.PCIVendors...) {
			if !isHex4(vendor) {
				return fmt.Errorf("pci vendor %q must be 4-digit hex without 0x", vendor)
			}
		}
		for _, dev := range append(spec.DeviceSelector.Include.PCIDevices, spec.DeviceSelector.Exclude.PCIDevices...) {
			if !isHex4(dev) {
				return fmt.Errorf("pci device %q must be 4-digit hex without 0x", dev)
			}
		}
		for _, mp := range append(spec.DeviceSelector.Include.MIGProfiles, spec.DeviceSelector.Exclude.MIGProfiles...) {
			if !isValidMIGProfile(mp) {
				return fmt.Errorf("migProfile %q has invalid format", mp)
			}
		}

		if spec.NodeSelector != nil {
			if _, err := metav1.LabelSelectorAsSelector(spec.NodeSelector); err != nil {
				return fmt.Errorf("invalid nodeSelector: %w", err)
			}
		}
		if sel := spec.DeviceAssignment.AutoApproveSelector; sel != nil {
			if _, err := metav1.LabelSelectorAsSelector(sel); err != nil {
				return fmt.Errorf("invalid deviceAssignment.autoApproveSelector: %w", err)
			}
		}
		return nil
	}
}
