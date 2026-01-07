//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package capabilities

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func buildCapabilities(dev NVMLDevice) (*gpuv1alpha1.GPUCapabilities, error) {
	name, ret := dev.GetName()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML device name failed: %s", nvml.ErrorString(ret))
	}

	mem, ret := dev.GetMemoryInfo()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML memory info failed: %s", nvml.ErrorString(ret))
	}

	major, minor, ret := dev.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML compute capability failed: %s", nvml.ErrorString(ret))
	}

	arch, ret := dev.GetArchitecture()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML architecture failed: %s", nvml.ErrorString(ret))
	}

	capabilities := &gpuv1alpha1.GPUCapabilities{
		ProductName: name,
		MemoryMiB:   int64Ptr(int64(mem.Total / (1024 * 1024))),
		Vendor:      gpuv1alpha1.VendorNvidia,
		Nvidia: &gpuv1alpha1.NvidiaCapabilities{
			ComputeCap:          fmt.Sprintf("%d.%d", major, minor),
			ProductArchitecture: architectureName(arch),
			ComputeTypes:        []string{"FP32", "FP64", "FP16", "BF16", "TF32", "INT8", "INT4", "FP8"},
		},
	}

	if boardPartNumber, ret := dev.GetBoardPartNumber(); ret == nvml.SUCCESS {
		capabilities.Nvidia.BoardPartNumber = boardPartNumber
	}

	if minLimit, maxLimit, ret := dev.GetPowerManagementLimitConstraints(); ret == nvml.SUCCESS {
		capabilities.Nvidia.PowerLimitMinW = int64Ptr(milliwattsToWatts(minLimit))
		capabilities.Nvidia.PowerLimitMaxW = int64Ptr(milliwattsToWatts(maxLimit))
	}

	migSupported, migCaps := readMigCapabilities(dev)
	if migSupported != nil {
		capabilities.Nvidia.MIGSupported = migSupported
		if *migSupported {
			capabilities.Nvidia.MIG = migCaps
		}
	}

	return capabilities, nil
}
