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

package physical

import (
	"fmt"
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	invcommon "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/common"
	invtypes "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/types"
)

// Builder creates physical GPU device offers.
type Builder struct{}

// NewBuilder returns a builder for physical GPU devices.
func NewBuilder() *Builder {
	return &Builder{}
}

// Build builds a physical GPU device offer.
func (b *Builder) Build(pgpu gpuv1alpha1.PhysicalGPU, _ invtypes.BuildContext) (invtypes.BuildResult, error) {
	device, err := buildPhysicalDevice(pgpu)
	if err != nil {
		return invtypes.BuildResult{}, err
	}
	return invtypes.BuildResult{Devices: allocatable.DeviceList{device}}, nil
}

func buildPhysicalDevice(pgpu gpuv1alpha1.PhysicalGPU) (allocatable.Device, error) {
	pciAddress := invcommon.PCIAddressFor(pgpu)
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

	attributes := invcommon.BaseAttributes(pgpu, gpuv1alpha1.DeviceTypePhysical, "")
	capacity := map[string]allocatable.CapacityValue{
		allocatable.CapMemory:       invcommon.ShareableMemoryCapacityMiB(*pgpu.Status.Capabilities.MemoryMiB),
		allocatable.CapSharePercent: invcommon.SharePercentCapacity(),
	}

	uuid := ""
	if pgpu.Status.CurrentState != nil && pgpu.Status.CurrentState.Nvidia != nil {
		uuid = strings.TrimSpace(pgpu.Status.CurrentState.Nvidia.GPUUUID)
	}

	return allocatable.NewGPUDevice(
		invcommon.DeviceName("gpu", pciAddress),
		uuid,
		attributes,
		capacity,
		nil,
		true,
	), nil
}
