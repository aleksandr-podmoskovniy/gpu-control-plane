/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func vendorAttribute(pgpu gpuv1alpha1.PhysicalGPU) string {
	if val := strings.TrimSpace(pgpu.Labels["gpu.deckhouse.io/vendor"]); val != "" {
		return val
	}
	if pgpu.Status.Capabilities != nil {
		switch pgpu.Status.Capabilities.Vendor {
		case gpuv1alpha1.VendorNvidia:
			return "nvidia"
		case gpuv1alpha1.VendorAMD:
			return "amd"
		case gpuv1alpha1.VendorIntel:
			return "intel"
		}
	}
	if pgpu.Status.PCIInfo != nil && pgpu.Status.PCIInfo.Vendor != nil {
		return normalizeLabelValue(pgpu.Status.PCIInfo.Vendor.Name)
	}
	return ""
}

func deviceAttribute(pgpu gpuv1alpha1.PhysicalGPU) string {
	if val := strings.TrimSpace(pgpu.Labels["gpu.deckhouse.io/device"]); val != "" {
		return val
	}
	if pgpu.Status.PCIInfo != nil && pgpu.Status.PCIInfo.Device != nil {
		return normalizeLabelValue(pgpu.Status.PCIInfo.Device.Name)
	}
	return ""
}

// PCIAddressFor returns the PCI address of the PhysicalGPU.
func PCIAddressFor(pgpu gpuv1alpha1.PhysicalGPU) string {
	if pgpu.Status.PCIInfo == nil {
		return ""
	}
	return strings.TrimSpace(pgpu.Status.PCIInfo.Address)
}

// DeviceName returns a DNS-safe device name for a PCI address.
func DeviceName(prefix, pciAddress string) string {
	return allocatable.SanitizeDNSLabel(prefix + "-" + pciAddress)
}
