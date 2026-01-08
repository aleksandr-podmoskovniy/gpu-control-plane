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
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

func (m *Manager) findExisting(device nvml.Device, profile nvml.GpuInstanceProfileInfo_v3, req domain.MigPrepareRequest) (domain.PreparedMigDevice, bool) {
	giInfo := nvml.GpuInstanceProfileInfo{Id: profile.Id}
	gpuInstances, ret := device.GetGpuInstances(&giInfo)
	if ret != nvml.SUCCESS || len(gpuInstances) == 0 {
		return domain.PreparedMigDevice{}, false
	}
	ciInfo, ret := computeProfileForSlices(profile.SliceCount, nil)
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, false
	}
	for _, gi := range gpuInstances {
		info, giRet := gi.GetInfo()
		if giRet != nvml.SUCCESS {
			continue
		}
		if int(info.Placement.Start) != req.SliceStart || int(info.Placement.Size) != req.SliceSize {
			continue
		}
		computeInstances, ciRet := gi.GetComputeInstances(&ciInfo)
		if ciRet != nvml.SUCCESS || len(computeInstances) == 0 {
			continue
		}
		computeInfo, ciInfoRet := computeInstances[0].GetInfo()
		if ciInfoRet != nvml.SUCCESS {
			continue
		}
		uuid, uuidRet := computeInfo.Device.GetUUID()
		if uuidRet != nvml.SUCCESS {
			continue
		}
		return domain.PreparedMigDevice{
			PCIBusID:          req.PCIBusID,
			ProfileID:         req.ProfileID,
			SliceStart:        req.SliceStart,
			SliceSize:         req.SliceSize,
			GPUInstanceID:     int(info.Id),
			ComputeInstanceID: int(computeInfo.Id),
			DeviceUUID:        uuid,
		}, true
	}
	return domain.PreparedMigDevice{}, false
}
