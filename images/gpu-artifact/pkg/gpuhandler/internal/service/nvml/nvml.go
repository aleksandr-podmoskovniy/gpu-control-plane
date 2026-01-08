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

package nvml

import nvmlapi "github.com/NVIDIA/go-nvml/pkg/nvml"

// NVML exposes only the NVML operations needed by the gpu-handler.
type NVML interface {
	Init() nvmlapi.Return
	Shutdown() nvmlapi.Return
	SystemGetDriverVersion() (string, nvmlapi.Return)
	SystemGetCudaDriverVersion() (int, nvmlapi.Return)
	DeviceByPCI(pciBusID string) (NVMLDevice, nvmlapi.Return)
	ErrorString(ret nvmlapi.Return) string
}

// NVMLDevice is a thin wrapper over nvmlapi.Device with only required methods.
type NVMLDevice interface {
	GetName() (string, nvmlapi.Return)
	GetUUID() (string, nvmlapi.Return)
	GetMemoryInfo() (nvmlapi.Memory, nvmlapi.Return)
	GetCudaComputeCapability() (int, int, nvmlapi.Return)
	GetArchitecture() (nvmlapi.DeviceArchitecture, nvmlapi.Return)
	GetBoardPartNumber() (string, nvmlapi.Return)
	GetPowerManagementLimit() (uint32, nvmlapi.Return)
	GetEnforcedPowerLimit() (uint32, nvmlapi.Return)
	GetPowerManagementLimitConstraints() (uint32, uint32, nvmlapi.Return)
	GetMigMode() (int, int, nvmlapi.Return)
	GetGpuInstanceProfileInfo(profile int) (nvmlapi.GpuInstanceProfileInfo, nvmlapi.Return)
	GetGpuInstanceProfileInfoV2(profile int) (nvmlapi.GpuInstanceProfileInfo_v2, nvmlapi.Return)
	GetGpuInstanceProfileInfoV3(profile int) (nvmlapi.GpuInstanceProfileInfo_v3, nvmlapi.Return)
	GetGpuInstancePossiblePlacements(info *nvmlapi.GpuInstanceProfileInfo) ([]nvmlapi.GpuInstancePlacement, nvmlapi.Return)
}
