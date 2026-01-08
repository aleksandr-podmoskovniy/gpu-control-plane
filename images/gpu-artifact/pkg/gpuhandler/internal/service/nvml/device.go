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

type nvmlDevice struct {
	device nvmlapi.Device
}

func (d nvmlDevice) GetName() (string, nvmlapi.Return) {
	return d.device.GetName()
}

func (d nvmlDevice) GetUUID() (string, nvmlapi.Return) {
	return d.device.GetUUID()
}

func (d nvmlDevice) GetMemoryInfo() (nvmlapi.Memory, nvmlapi.Return) {
	return d.device.GetMemoryInfo()
}

func (d nvmlDevice) GetCudaComputeCapability() (int, int, nvmlapi.Return) {
	return d.device.GetCudaComputeCapability()
}

func (d nvmlDevice) GetArchitecture() (nvmlapi.DeviceArchitecture, nvmlapi.Return) {
	return d.device.GetArchitecture()
}

func (d nvmlDevice) GetBoardPartNumber() (string, nvmlapi.Return) {
	return d.device.GetBoardPartNumber()
}

func (d nvmlDevice) GetPowerManagementLimit() (uint32, nvmlapi.Return) {
	return d.device.GetPowerManagementLimit()
}

func (d nvmlDevice) GetEnforcedPowerLimit() (uint32, nvmlapi.Return) {
	return d.device.GetEnforcedPowerLimit()
}

func (d nvmlDevice) GetPowerManagementLimitConstraints() (uint32, uint32, nvmlapi.Return) {
	return d.device.GetPowerManagementLimitConstraints()
}

func (d nvmlDevice) GetMigMode() (int, int, nvmlapi.Return) {
	return d.device.GetMigMode()
}

func (d nvmlDevice) GetGpuInstanceProfileInfo(profile int) (nvmlapi.GpuInstanceProfileInfo, nvmlapi.Return) {
	return d.device.GetGpuInstanceProfileInfo(profile)
}

func (d nvmlDevice) GetGpuInstanceProfileInfoV2(profile int) (nvmlapi.GpuInstanceProfileInfo_v2, nvmlapi.Return) {
	return d.device.GetGpuInstanceProfileInfoV(profile).V2()
}

func (d nvmlDevice) GetGpuInstanceProfileInfoV3(profile int) (nvmlapi.GpuInstanceProfileInfo_v3, nvmlapi.Return) {
	return d.device.GetGpuInstanceProfileInfoV(profile).V3()
}

func (d nvmlDevice) GetGpuInstancePossiblePlacements(info *nvmlapi.GpuInstanceProfileInfo) ([]nvmlapi.GpuInstancePlacement, nvmlapi.Return) {
	return d.device.GetGpuInstancePossiblePlacements(info)
}
