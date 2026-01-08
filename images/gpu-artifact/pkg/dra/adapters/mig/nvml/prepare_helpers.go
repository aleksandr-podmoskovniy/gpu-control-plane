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

package mig

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

func ensureMigMode(lib nvml.Interface, device nvml.Device) error {
	current, _, ret := lib.DeviceGetMigMode(device)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("get mig mode: %s", nvml.ErrorString(ret))
	}
	if current == nvml.DEVICE_MIG_ENABLE {
		return nil
	}
	activation, setRet := lib.DeviceSetMigMode(device, nvml.DEVICE_MIG_ENABLE)
	if setRet != nvml.SUCCESS {
		return fmt.Errorf("enable mig mode: %s", nvml.ErrorString(setRet))
	}
	if activation != nvml.SUCCESS {
		return fmt.Errorf("mig mode activation pending: %s", nvml.ErrorString(activation))
	}
	return nil
}

func lookupProfile(device nvml.Device, profileID int) (nvml.GpuInstanceProfileInfo_v3, error) {
	profileInfo, ret := device.GetGpuInstanceProfileInfoByIdV(profileID).V3()
	if ret != nvml.SUCCESS {
		return nvml.GpuInstanceProfileInfo_v3{}, fmt.Errorf("get mig profile info: %s", nvml.ErrorString(ret))
	}
	return profileInfo, nil
}

func createMigDevice(lib nvml.Interface, device nvml.Device, profileInfo nvml.GpuInstanceProfileInfo_v3, req domain.MigPrepareRequest) (domain.PreparedMigDevice, error) {
	giInfo := nvml.GpuInstanceProfileInfo{Id: profileInfo.Id}
	giPlacement := nvml.GpuInstancePlacement{Start: uint32(req.SliceStart), Size: uint32(req.SliceSize)}
	gpuInstance, ret := device.CreateGpuInstanceWithPlacement(&giInfo, &giPlacement)
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, fmt.Errorf("create gpu instance: %s", nvml.ErrorString(ret))
	}

	ciInfo, ret := computeProfileForSlices(profileInfo.SliceCount, gpuInstance)
	if ret != nvml.SUCCESS {
		_ = lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get compute profile: %s", nvml.ErrorString(ret))
	}
	ciPlacement := selectComputePlacement(ciInfo)
	computeInstance, ret := gpuInstance.CreateComputeInstanceWithPlacement(&ciInfo, &ciPlacement)
	if ret != nvml.SUCCESS {
		_ = lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("create compute instance: %s", nvml.ErrorString(ret))
	}

	gpuInfo, ret := lib.GpuInstanceGetInfo(gpuInstance)
	if ret != nvml.SUCCESS {
		_ = lib.ComputeInstanceDestroy(computeInstance)
		_ = lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get gpu instance info: %s", nvml.ErrorString(ret))
	}
	computeInfo, ret := lib.ComputeInstanceGetInfo(computeInstance)
	if ret != nvml.SUCCESS {
		_ = lib.ComputeInstanceDestroy(computeInstance)
		_ = lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get compute instance info: %s", nvml.ErrorString(ret))
	}
	uuid, ret := lib.DeviceGetUUID(computeInfo.Device)
	if ret != nvml.SUCCESS {
		_ = lib.ComputeInstanceDestroy(computeInstance)
		_ = lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get mig uuid: %s", nvml.ErrorString(ret))
	}

	return domain.PreparedMigDevice{
		PCIBusID:          req.PCIBusID,
		ProfileID:         req.ProfileID,
		SliceStart:        req.SliceStart,
		SliceSize:         req.SliceSize,
		GPUInstanceID:     int(gpuInfo.Id),
		ComputeInstanceID: int(computeInfo.Id),
		DeviceUUID:        uuid,
	}, nil
}
