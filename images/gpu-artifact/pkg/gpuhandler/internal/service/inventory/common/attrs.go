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
