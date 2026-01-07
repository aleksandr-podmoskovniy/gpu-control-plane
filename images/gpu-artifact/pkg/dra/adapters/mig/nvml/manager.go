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
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/nvmlutil"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Manager prepares MIG devices using NVML.
type Manager struct {
	lib nvml.Interface
}

// Options configure the MIG manager.
type Options struct {
	DriverRoot string
}

// New constructs a new MIG manager.
func New(opts Options) *Manager {
	return &Manager{lib: nvml.New(nvmlutil.LibraryOptions(opts.DriverRoot)...)}
}

// Prepare creates a MIG instance for the requested placement.
func (m *Manager) Prepare(_ context.Context, req domain.MigPrepareRequest) (domain.PreparedMigDevice, error) {
	if m == nil || m.lib == nil {
		return domain.PreparedMigDevice{}, errors.New("nvml is not configured")
	}
	if req.PCIBusID == "" {
		return domain.PreparedMigDevice{}, errors.New("pci bus id is required")
	}
	if req.SliceSize <= 0 {
		return domain.PreparedMigDevice{}, errors.New("mig placement size is required")
	}

	if err := initNVML(m.lib); err != nil {
		return domain.PreparedMigDevice{}, err
	}
	defer m.lib.Shutdown()

	device, ret := m.lib.DeviceGetHandleByPciBusId(req.PCIBusID)
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, fmt.Errorf("get device by pci %q: %s", req.PCIBusID, nvml.ErrorString(ret))
	}

	current, _, ret := m.lib.DeviceGetMigMode(device)
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, fmt.Errorf("get mig mode: %s", nvml.ErrorString(ret))
	}
	if current != nvml.DEVICE_MIG_ENABLE {
		activation, setRet := m.lib.DeviceSetMigMode(device, nvml.DEVICE_MIG_ENABLE)
		if setRet != nvml.SUCCESS {
			return domain.PreparedMigDevice{}, fmt.Errorf("enable mig mode: %s", nvml.ErrorString(setRet))
		}
		if activation != nvml.SUCCESS {
			return domain.PreparedMigDevice{}, fmt.Errorf("mig mode activation pending: %s", nvml.ErrorString(activation))
		}
	}

	profileInfo, ret := m.lib.DeviceGetGpuInstanceProfileInfoByIdV(device, req.ProfileID).V3()
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, fmt.Errorf("get mig profile info: %s", nvml.ErrorString(ret))
	}

	existing, ok := m.findExisting(device, profileInfo, req)
	if ok {
		return existing, nil
	}

	giInfo := nvml.GpuInstanceProfileInfo{Id: profileInfo.Id}
	giPlacement := nvml.GpuInstancePlacement{Start: uint32(req.SliceStart), Size: uint32(req.SliceSize)}
	gpuInstance, ret := device.CreateGpuInstanceWithPlacement(&giInfo, &giPlacement)
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, fmt.Errorf("create gpu instance: %s", nvml.ErrorString(ret))
	}

	ciInfo, ret := computeProfileForSlices(profileInfo.SliceCount, gpuInstance)
	if ret != nvml.SUCCESS {
		_ = m.lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get compute profile: %s", nvml.ErrorString(ret))
	}
	ciPlacement := selectComputePlacement(ciInfo)
	computeInstance, ret := gpuInstance.CreateComputeInstanceWithPlacement(&ciInfo, &ciPlacement)
	if ret != nvml.SUCCESS {
		_ = m.lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("create compute instance: %s", nvml.ErrorString(ret))
	}

	gpuInfo, ret := m.lib.GpuInstanceGetInfo(gpuInstance)
	if ret != nvml.SUCCESS {
		_ = m.lib.ComputeInstanceDestroy(computeInstance)
		_ = m.lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get gpu instance info: %s", nvml.ErrorString(ret))
	}
	computeInfo, ret := m.lib.ComputeInstanceGetInfo(computeInstance)
	if ret != nvml.SUCCESS {
		_ = m.lib.ComputeInstanceDestroy(computeInstance)
		_ = m.lib.GpuInstanceDestroy(gpuInstance)
		return domain.PreparedMigDevice{}, fmt.Errorf("get compute instance info: %s", nvml.ErrorString(ret))
	}
	uuid, ret := m.lib.DeviceGetUUID(computeInfo.Device)
	if ret != nvml.SUCCESS {
		_ = m.lib.ComputeInstanceDestroy(computeInstance)
		_ = m.lib.GpuInstanceDestroy(gpuInstance)
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

// Unprepare removes a MIG instance.
func (m *Manager) Unprepare(_ context.Context, state domain.PreparedMigDevice) error {
	if m == nil || m.lib == nil {
		return errors.New("nvml is not configured")
	}
	if state.PCIBusID == "" {
		return errors.New("pci bus id is required")
	}
	if err := initNVML(m.lib); err != nil {
		return err
	}
	defer m.lib.Shutdown()

	device, ret := m.lib.DeviceGetHandleByPciBusId(state.PCIBusID)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("get device by pci %q: %s", state.PCIBusID, nvml.ErrorString(ret))
	}

	gpuInstance, ret := m.lib.DeviceGetGpuInstanceById(device, state.GPUInstanceID)
	if ret != nvml.SUCCESS {
		if ret == nvml.ERROR_NOT_FOUND {
			return nil
		}
		return fmt.Errorf("get gpu instance: %s", nvml.ErrorString(ret))
	}
	computeInstance, ret := m.lib.GpuInstanceGetComputeInstanceById(gpuInstance, state.ComputeInstanceID)
	if ret == nvml.SUCCESS {
		if destroy := m.lib.ComputeInstanceDestroy(computeInstance); destroy != nvml.SUCCESS {
			return fmt.Errorf("destroy compute instance: %s", nvml.ErrorString(destroy))
		}
	}
	if ret != nvml.SUCCESS && ret != nvml.ERROR_NOT_FOUND {
		return fmt.Errorf("get compute instance: %s", nvml.ErrorString(ret))
	}

	if destroy := m.lib.GpuInstanceDestroy(gpuInstance); destroy != nvml.SUCCESS && destroy != nvml.ERROR_NOT_FOUND {
		return fmt.Errorf("destroy gpu instance: %s", nvml.ErrorString(destroy))
	}
	return nil
}

func initNVML(lib nvml.Interface) error {
	ret := lib.Init()
	if ret == nvml.SUCCESS || ret == nvml.ERROR_ALREADY_INITIALIZED {
		return nil
	}
	return fmt.Errorf("NVML init failed: %s", nvml.ErrorString(ret))
}

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

func computeProfileForSlices(sliceCount uint32, gpuInstance nvml.GpuInstance) (nvml.ComputeInstanceProfileInfo, nvml.Return) {
	profile, ok := computeProfileBySliceCount(sliceCount)
	if !ok {
		return nvml.ComputeInstanceProfileInfo{}, nvml.ERROR_INVALID_ARGUMENT
	}
	if gpuInstance == nil {
		return nvml.ComputeInstanceProfileInfo{Id: uint32(profile)}, nvml.SUCCESS
	}
	return gpuInstance.GetComputeInstanceProfileInfo(profile, nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_SHARED)
}

func computeProfileBySliceCount(sliceCount uint32) (int, bool) {
	switch sliceCount {
	case 1:
		return nvml.COMPUTE_INSTANCE_PROFILE_1_SLICE, true
	case 2:
		return nvml.COMPUTE_INSTANCE_PROFILE_2_SLICE, true
	case 3:
		return nvml.COMPUTE_INSTANCE_PROFILE_3_SLICE, true
	case 4:
		return nvml.COMPUTE_INSTANCE_PROFILE_4_SLICE, true
	case 6:
		return nvml.COMPUTE_INSTANCE_PROFILE_6_SLICE, true
	case 7:
		return nvml.COMPUTE_INSTANCE_PROFILE_7_SLICE, true
	case 8:
		return nvml.COMPUTE_INSTANCE_PROFILE_8_SLICE, true
	default:
		return 0, false
	}
}

func selectComputePlacement(info nvml.ComputeInstanceProfileInfo) nvml.ComputeInstancePlacement {
	return nvml.ComputeInstancePlacement{Start: 0, Size: info.SliceCount}
}
