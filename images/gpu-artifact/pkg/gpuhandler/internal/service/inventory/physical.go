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

package inventory

import (
	"fmt"
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

type PhysicalDeviceBuilder struct{}

// NewPhysicalDeviceBuilder returns a builder for physical GPU devices.
func NewPhysicalDeviceBuilder() *PhysicalDeviceBuilder {
	return &PhysicalDeviceBuilder{}
}

// Build builds a physical GPU device offer.
func (b *PhysicalDeviceBuilder) Build(pgpu gpuv1alpha1.PhysicalGPU, _ BuildContext) (BuildResult, error) {
	device, err := buildPhysicalDevice(pgpu)
	if err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Devices: allocatable.DeviceList{device}}, nil
}

func buildPhysicalDevice(pgpu gpuv1alpha1.PhysicalGPU) (allocatable.Device, error) {
	pciAddress := pciAddressFor(pgpu)
	if pciAddress == "" {
		return nil, fmt.Errorf("missing pci address for %s", pgpu.Name)
	}
	if pgpu.Status.Capabilities == nil {
		return nil, fmt.Errorf("missing capabilities for %s", pgpu.Name)
	}
	if pgpu.Status.Capabilities.Vendor != gpuv1alpha1.VendorNvidia {
		return nil, fmt.Errorf("unsupported vendor for %s", pgpu.Name)
	}
	if pgpu.Status.Capabilities.MemoryMiB == nil {
		return nil, fmt.Errorf("missing memoryMiB for %s", pgpu.Name)
	}

	attributes := baseAttributes(pgpu, gpuv1alpha1.DeviceTypePhysical, "")
	capacity := map[string]allocatable.CapacityValue{
		allocatable.CapMemory:       shareableMemoryCapacityMiB(*pgpu.Status.Capabilities.MemoryMiB),
		allocatable.CapSharePercent: sharePercentCapacity(),
	}

	uuid := ""
	if pgpu.Status.CurrentState != nil && pgpu.Status.CurrentState.Nvidia != nil {
		uuid = strings.TrimSpace(pgpu.Status.CurrentState.Nvidia.GPUUUID)
	}

	return allocatable.NewGPUDevice(
		deviceName("gpu", pciAddress),
		uuid,
		attributes,
		capacity,
		nil,
		true,
	), nil
}
