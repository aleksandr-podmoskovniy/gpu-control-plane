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
	"fmt"
	"strconv"
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// BaseAttributes builds common attributes for device offers.
func BaseAttributes(pgpu gpuv1alpha1.PhysicalGPU, deviceType gpuv1alpha1.DeviceType, migProfile string) map[string]allocatable.AttributeValue {
	attrs := map[string]allocatable.AttributeValue{}

	if vendor := vendorAttribute(pgpu); vendor != "" {
		attrs[allocatable.AttrVendor] = stringAttr(vendor)
	}
	if device := deviceAttribute(pgpu); device != "" {
		attrs[allocatable.AttrDevice] = stringAttr(device)
	}
	attrs[allocatable.AttrDeviceType] = stringAttr(string(deviceType))

	if pci := PCIAddressFor(pgpu); pci != "" {
		attrs[allocatable.AttrPCIAddress] = stringAttr(pci)
	}

	if pgpu.Status.CurrentState != nil && pgpu.Status.CurrentState.Nvidia != nil {
		if uuid := strings.TrimSpace(pgpu.Status.CurrentState.Nvidia.GPUUUID); uuid != "" {
			attrs[allocatable.AttrGPUUUID] = stringAttr(uuid)
		}
		if version := strings.TrimSpace(pgpu.Status.CurrentState.Nvidia.DriverVersion); version != "" {
			attrs[allocatable.AttrDriverVer] = stringAttr(version)
		}
	}

	if pgpu.Status.Capabilities != nil && pgpu.Status.Capabilities.Nvidia != nil {
		if major, minor, ok := parseComputeCap(pgpu.Status.Capabilities.Nvidia.ComputeCap); ok {
			attrs[allocatable.AttrCCMajor] = intAttr(int64(major))
			attrs[allocatable.AttrCCMinor] = intAttr(int64(minor))
		}
	}

	if migProfile != "" {
		attrs[allocatable.AttrMigProfile] = stringAttr(migProfile)
	}

	return attrs
}

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
	return allocatable.SanitizeDNSLabel(fmt.Sprintf("%s-%s", prefix, pciAddress))
}

func parseComputeCap(raw string) (int, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(raw, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	if len(parts) == 1 {
		return major, 0, true
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func stringAttr(val string) allocatable.AttributeValue {
	return allocatable.AttributeValue{String: &val}
}

func intAttr(val int64) allocatable.AttributeValue {
	return allocatable.AttributeValue{Int: &val}
}

func normalizeLabelValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
