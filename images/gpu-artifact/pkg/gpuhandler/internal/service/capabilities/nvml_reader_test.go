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
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func TestNVMLReaderSuccess(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:          "NVIDIA A30",
		uuid:          "GPU-123",
		memory:        nvml.Memory{Total: 24576 * 1024 * 1024},
		major:         8,
		minor:         0,
		arch:          nvml.DEVICE_ARCH_AMPERE,
		boardPart:     "900-21001-0040-100",
		powerLimit:    165000,
		enforcedPower: 165000,
		minPower:      100000,
		maxPower:      200000,
		migRet:        nvml.ERROR_NOT_SUPPORTED,
		profileRet:    nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "580.76.05",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if snapshot.Capabilities == nil || snapshot.Capabilities.ProductName != "NVIDIA A30" {
		t.Fatalf("capabilities not populated")
	}
	if snapshot.Capabilities.Nvidia == nil || snapshot.Capabilities.Nvidia.ProductArchitecture != "Ampere" {
		t.Fatalf("nvidia capabilities missing")
	}
	if snapshot.Capabilities.Nvidia.MIGSupported == nil || *snapshot.Capabilities.Nvidia.MIGSupported {
		t.Fatalf("expected MIGSupported false")
	}
	if snapshot.Capabilities.Nvidia.MIG != nil {
		t.Fatalf("expected MIG capabilities to be empty when unsupported")
	}
	if snapshot.CurrentState == nil || snapshot.CurrentState.Nvidia == nil {
		t.Fatalf("current state missing")
	}
	if snapshot.CurrentState.Nvidia.CUDAVersion != "13.0" {
		t.Fatalf("unexpected cuda version: %s", snapshot.CurrentState.Nvidia.CUDAVersion)
	}
	if snapshot.CurrentState.DriverType != "" {
		t.Fatalf("driver type should not be set by reader")
	}
	if snapshot.Capabilities.Vendor != gpuv1alpha1.VendorNvidia {
		t.Fatalf("expected vendor Nvidia")
	}
}

func TestNVMLReaderMIGProfiles(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:      "NVIDIA A30",
		uuid:      "GPU-123",
		memory:    nvml.Memory{Total: 24576 * 1024 * 1024},
		major:     8,
		minor:     0,
		arch:      nvml.DEVICE_ARCH_AMPERE,
		boardPart: "900-21001-0040-100",
		migMode:   nvml.DEVICE_MIG_ENABLE,
		migRet:    nvml.SUCCESS,
		profileInfo: map[int]nvml.GpuInstanceProfileInfo_v3{
			5: profileInfo(5, 2, 2, 12032, "2g.12gb"),
			0: profileInfo(0, 4, 1, 24125, "4g.24gb"),
		},
		profileRet: nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "580.76.05",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if snapshot.Capabilities == nil || snapshot.Capabilities.Nvidia == nil {
		t.Fatalf("capabilities missing")
	}
	if snapshot.Capabilities.Nvidia.MIGSupported == nil || !*snapshot.Capabilities.Nvidia.MIGSupported {
		t.Fatalf("expected MIGSupported true")
	}
	if snapshot.Capabilities.Nvidia.MIG == nil {
		t.Fatalf("expected MIG capabilities")
	}
	if snapshot.Capabilities.Nvidia.MIG.TotalSlices != 4 {
		t.Fatalf("expected totalSlices 4, got %d", snapshot.Capabilities.Nvidia.MIG.TotalSlices)
	}
	if len(snapshot.Capabilities.Nvidia.MIG.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(snapshot.Capabilities.Nvidia.MIG.Profiles))
	}
}

func TestNVMLReaderMIGProfilesSuffixByProfileID(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:      "NVIDIA A30",
		uuid:      "GPU-123",
		memory:    nvml.Memory{Total: 24576 * 1024 * 1024},
		major:     8,
		minor:     0,
		arch:      nvml.DEVICE_ARCH_AMPERE,
		boardPart: "900-21001-0040-100",
		migMode:   nvml.DEVICE_MIG_ENABLE,
		migRet:    nvml.SUCCESS,
		profileInfo: map[int]nvml.GpuInstanceProfileInfo_v3{
			0: profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE, 1, 4, 5952, "MIG 1g.6gb", 1, 0, 0, 0, 0),
			1: profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1, 1, 1, 5952, "MIG 1g.6gb+me", 0, 0, 0, 0, 0),
			2: profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE_ALL_ME, 1, 1, 5952, "MIG 1g.6gb+me.all", 0, 0, 0, 0, 0),
			3: profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE_GFX, 1, 1, 5952, "MIG 1g.6gb+gfx", 0, 0, 0, 0, nvml.GPU_INSTANCE_PROFILE_CAPS_GFX),
			4: profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE_NO_ME, 1, 1, 5952, "MIG 1g.6gb-me", 1, 0, 0, 0, 0),
		},
		profileRet: nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "590.48.01",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	profiles := snapshot.Capabilities.Nvidia.MIG.Profiles
	names := map[int32]string{}
	for _, profile := range profiles {
		names[profile.ProfileID] = profile.Name
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE] != "1g.6gb" {
		t.Fatalf("expected profile 1_slice name 1g.6gb, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE])
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1] != "1g.6gb+me" {
		t.Fatalf("expected profile 1_slice_rev1 name 1g.6gb+me, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1])
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_ALL_ME] != "1g.6gb+me.all" {
		t.Fatalf("expected profile 1_slice_all_me name 1g.6gb+me.all, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_ALL_ME])
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_GFX] != "1g.6gb+gfx" {
		t.Fatalf("expected profile 1_slice_gfx name 1g.6gb+gfx, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_GFX])
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_NO_ME] != "1g.6gb-me" {
		t.Fatalf("expected profile 1_slice_no_me name 1g.6gb-me, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_NO_ME])
	}
}

func TestNVMLReaderMIGProfilesNameFromNVML(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:      "NVIDIA A30",
		uuid:      "GPU-123",
		memory:    nvml.Memory{Total: 24576 * 1024 * 1024},
		major:     8,
		minor:     0,
		arch:      nvml.DEVICE_ARCH_AMPERE,
		boardPart: "900-21001-0040-100",
		migMode:   nvml.DEVICE_MIG_ENABLE,
		migRet:    nvml.SUCCESS,
		profileInfo: map[int]nvml.GpuInstanceProfileInfo_v3{
			int(nvml.GPU_INSTANCE_PROFILE_1_SLICE):      profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE, 1, 4, 5952, "MIG 1g.6gb", 1, 0, 0, 0, 0),
			int(nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1): profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1, 1, 1, 5952, "MIG 1g.6gb", 0, 0, 0, 0, 0),
		},
		profileRet: nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "590.48.01",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	profiles := snapshot.Capabilities.Nvidia.MIG.Profiles
	names := map[int32]string{}
	for _, profile := range profiles {
		names[profile.ProfileID] = profile.Name
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE] != "1g.6gb" {
		t.Fatalf("expected profile 1_slice name 1g.6gb, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE])
	}
	if names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1] != "1g.6gb" {
		t.Fatalf("expected profile 1_slice_rev1 name 1g.6gb, got %q", names[nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1])
	}
}

func TestNVMLReaderMIGProfilesFallbackV2(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:      "NVIDIA A30",
		uuid:      "GPU-123",
		memory:    nvml.Memory{Total: 24576 * 1024 * 1024},
		major:     8,
		minor:     0,
		arch:      nvml.DEVICE_ARCH_AMPERE,
		boardPart: "900-21001-0040-100",
		migMode:   nvml.DEVICE_MIG_ENABLE,
		migRet:    nvml.SUCCESS,
		profileInfoV2: map[int]nvml.GpuInstanceProfileInfo_v2{
			5: profileInfoV2(5, 2, 2, 12032, "2g.12gb"),
			0: profileInfoV2(0, 4, 1, 24125, "4g.24gb"),
		},
		profileRet: nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "580.76.05",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if snapshot.Capabilities == nil || snapshot.Capabilities.Nvidia == nil {
		t.Fatalf("capabilities missing")
	}
	if snapshot.Capabilities.Nvidia.MIGSupported == nil || !*snapshot.Capabilities.Nvidia.MIGSupported {
		t.Fatalf("expected MIGSupported true")
	}
	if snapshot.Capabilities.Nvidia.MIG == nil {
		t.Fatalf("expected MIG capabilities")
	}
	if snapshot.Capabilities.Nvidia.MIG.TotalSlices != 4 {
		t.Fatalf("expected totalSlices 4, got %d", snapshot.Capabilities.Nvidia.MIG.TotalSlices)
	}
	if len(snapshot.Capabilities.Nvidia.MIG.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(snapshot.Capabilities.Nvidia.MIG.Profiles))
	}
}

func TestNVMLReaderMIGProfilesFallbackNameFromV2(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:      "NVIDIA A30",
		uuid:      "GPU-123",
		memory:    nvml.Memory{Total: 24576 * 1024 * 1024},
		major:     8,
		minor:     0,
		arch:      nvml.DEVICE_ARCH_AMPERE,
		boardPart: "900-21001-0040-100",
		migMode:   nvml.DEVICE_MIG_ENABLE,
		migRet:    nvml.SUCCESS,
		profileInfo: map[int]nvml.GpuInstanceProfileInfo_v3{
			0: profileInfoWithEnginesAndCaps(nvml.GPU_INSTANCE_PROFILE_1_SLICE, 1, 4, 5952, "", 1, 0, 0, 0, 0),
		},
		profileInfoV2: map[int]nvml.GpuInstanceProfileInfo_v2{
			0: profileInfoV2(nvml.GPU_INSTANCE_PROFILE_1_SLICE, 1, 4, 5952, "MIG 1g.6gb+me"),
		},
		profileRet: nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "590.48.01",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	profiles := snapshot.Capabilities.Nvidia.MIG.Profiles
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "1g.6gb+me" {
		t.Fatalf("expected profile name 1g.6gb+me, got %q", profiles[0].Name)
	}
}

func TestNVMLReaderPartialData(t *testing.T) {
	dev := &fakeNVMLDevice{
		name:           "NVIDIA A30",
		uuid:           "GPU-123",
		memory:         nvml.Memory{Total: 24576 * 1024 * 1024},
		major:          8,
		minor:          0,
		arch:           nvml.DEVICE_ARCH_AMPERE,
		boardPart:      "900-21001-0040-100",
		boardRet:       nvml.ERROR_NOT_SUPPORTED,
		constraintsRet: nvml.ERROR_NOT_SUPPORTED,
		migRet:         nvml.ERROR_NOT_SUPPORTED,
		profileRet:     nvml.ERROR_NOT_SUPPORTED,
	}

	reader := NewNVMLReader(&fakeNVML{
		initRet:       nvml.SUCCESS,
		driverVersion: "580.76.05",
		driverRet:     nvml.SUCCESS,
		cudaVersion:   13000,
		cudaRet:       nvml.SUCCESS,
		deviceRet:     nvml.SUCCESS,
		device:        dev,
	})

	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	snapshot, err := session.ReadDevice("0000:02:00.0")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if snapshot.Capabilities == nil || snapshot.Capabilities.Nvidia == nil {
		t.Fatalf("capabilities missing")
	}
	if snapshot.Capabilities.Nvidia.BoardPartNumber != "" {
		t.Fatalf("expected empty board part number")
	}
	if snapshot.Capabilities.Nvidia.PowerLimitMinW != nil || snapshot.Capabilities.Nvidia.PowerLimitMaxW != nil {
		t.Fatalf("expected power limits to be nil")
	}
}

func TestNVMLReaderMissingPCI(t *testing.T) {
	reader := NewNVMLReader(&fakeNVML{initRet: nvml.SUCCESS})
	session, err := reader.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()

	_, err = session.ReadDevice("")
	if !errors.Is(err, ErrMissingPCIAddress) {
		t.Fatalf("expected ErrMissingPCIAddress, got %v", err)
	}
}

func TestNVMLReaderInitFailure(t *testing.T) {
	reader := NewNVMLReader(&fakeNVML{initRet: nvml.ERROR_LIBRARY_NOT_FOUND})
	_, err := reader.Open()
	if !errors.Is(err, ErrNVMLUnavailable) {
		t.Fatalf("expected ErrNVMLUnavailable, got %v", err)
	}
}

type fakeNVML struct {
	initRet       nvml.Return
	driverVersion string
	driverRet     nvml.Return
	cudaVersion   int
	cudaRet       nvml.Return
	device        NVMLDevice
	deviceRet     nvml.Return
}

func (f *fakeNVML) Init() nvml.Return {
	return f.initRet
}

func (f *fakeNVML) Shutdown() nvml.Return {
	return nvml.SUCCESS
}

func (f *fakeNVML) SystemGetDriverVersion() (string, nvml.Return) {
	return f.driverVersion, f.driverRet
}

func (f *fakeNVML) SystemGetCudaDriverVersion() (int, nvml.Return) {
	return f.cudaVersion, f.cudaRet
}

func (f *fakeNVML) DeviceByPCI(_ string) (NVMLDevice, nvml.Return) {
	return f.device, f.deviceRet
}

func (f *fakeNVML) ErrorString(ret nvml.Return) string {
	return ret.String()
}

type fakeNVMLDevice struct {
	name           string
	uuid           string
	memory         nvml.Memory
	major          int
	minor          int
	arch           nvml.DeviceArchitecture
	boardPart      string
	boardRet       nvml.Return
	powerLimit     uint32
	enforcedPower  uint32
	minPower       uint32
	maxPower       uint32
	constraintsRet nvml.Return
	migMode        int
	migRet         nvml.Return
	profileInfo    map[int]nvml.GpuInstanceProfileInfo_v3
	profileInfoV2  map[int]nvml.GpuInstanceProfileInfo_v2
	profileRet     nvml.Return
	placements     map[int][]nvml.GpuInstancePlacement
	placementsRet  nvml.Return
}

func (d *fakeNVMLDevice) GetName() (string, nvml.Return) {
	return d.name, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetUUID() (string, nvml.Return) {
	return d.uuid, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetMemoryInfo() (nvml.Memory, nvml.Return) {
	return d.memory, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetCudaComputeCapability() (int, int, nvml.Return) {
	return d.major, d.minor, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetArchitecture() (nvml.DeviceArchitecture, nvml.Return) {
	return d.arch, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetBoardPartNumber() (string, nvml.Return) {
	if d.boardRet != 0 {
		return d.boardPart, d.boardRet
	}
	return d.boardPart, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetPowerManagementLimit() (uint32, nvml.Return) {
	return d.powerLimit, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetEnforcedPowerLimit() (uint32, nvml.Return) {
	return d.enforcedPower, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetPowerManagementLimitConstraints() (uint32, uint32, nvml.Return) {
	if d.constraintsRet != 0 {
		return 0, 0, d.constraintsRet
	}
	return d.minPower, d.maxPower, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetMigMode() (int, int, nvml.Return) {
	return d.migMode, 0, d.migRet
}

func (d *fakeNVMLDevice) GetGpuInstanceProfileInfo(profile int) (nvml.GpuInstanceProfileInfo, nvml.Return) {
	if info, ok := d.profileInfo[profile]; ok {
		return nvml.GpuInstanceProfileInfo{
			Id:            info.Id,
			SliceCount:    info.SliceCount,
			InstanceCount: info.InstanceCount,
			MemorySizeMB:  info.MemorySizeMB,
		}, nvml.SUCCESS
	}
	if info, ok := d.profileInfoV2[profile]; ok {
		return nvml.GpuInstanceProfileInfo{
			Id:            info.Id,
			SliceCount:    info.SliceCount,
			InstanceCount: info.InstanceCount,
			MemorySizeMB:  info.MemorySizeMB,
		}, nvml.SUCCESS
	}
	if d.profileRet != 0 {
		return nvml.GpuInstanceProfileInfo{}, d.profileRet
	}
	return nvml.GpuInstanceProfileInfo{}, nvml.ERROR_NOT_SUPPORTED
}

func (d *fakeNVMLDevice) GetGpuInstanceProfileInfoV2(profile int) (nvml.GpuInstanceProfileInfo_v2, nvml.Return) {
	if info, ok := d.profileInfoV2[profile]; ok {
		return info, nvml.SUCCESS
	}
	if info, ok := d.profileInfo[profile]; ok {
		return nvml.GpuInstanceProfileInfo_v2{
			Id:            info.Id,
			SliceCount:    info.SliceCount,
			InstanceCount: info.InstanceCount,
			MemorySizeMB:  info.MemorySizeMB,
			Name:          info.Name,
		}, nvml.SUCCESS
	}
	if d.profileRet != 0 {
		return nvml.GpuInstanceProfileInfo_v2{}, d.profileRet
	}
	return nvml.GpuInstanceProfileInfo_v2{}, nvml.ERROR_NOT_SUPPORTED
}

func (d *fakeNVMLDevice) GetGpuInstanceProfileInfoV3(profile int) (nvml.GpuInstanceProfileInfo_v3, nvml.Return) {
	if info, ok := d.profileInfo[profile]; ok {
		return info, nvml.SUCCESS
	}
	if d.profileRet != 0 {
		return nvml.GpuInstanceProfileInfo_v3{}, d.profileRet
	}
	return nvml.GpuInstanceProfileInfo_v3{}, nvml.ERROR_NOT_SUPPORTED
}

func (d *fakeNVMLDevice) GetGpuInstancePossiblePlacements(info *nvml.GpuInstanceProfileInfo) ([]nvml.GpuInstancePlacement, nvml.Return) {
	if info == nil {
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}
	if d.placementsRet != 0 {
		return nil, d.placementsRet
	}
	if placements, ok := d.placements[int(info.Id)]; ok {
		return placements, nvml.SUCCESS
	}
	return nil, nvml.ERROR_NOT_SUPPORTED
}

func profileInfo(id, sliceCount, instances, memoryMiB uint32, name string) nvml.GpuInstanceProfileInfo_v3 {
	var info nvml.GpuInstanceProfileInfo_v3
	info.Id = id
	info.SliceCount = sliceCount
	info.InstanceCount = instances
	info.MemorySizeMB = uint64(memoryMiB)
	copy(info.Name[:], []byte(name))
	return info
}

func profileInfoWithEnginesAndCaps(id, sliceCount, instances, memoryMiB uint32, name string, decoder, encoder, jpeg, ofa, caps uint32) nvml.GpuInstanceProfileInfo_v3 {
	info := profileInfo(id, sliceCount, instances, memoryMiB, name)
	info.DecoderCount = decoder
	info.EncoderCount = encoder
	info.JpegCount = jpeg
	info.OfaCount = ofa
	info.Capabilities = caps
	return info
}

func profileInfoV2(id, sliceCount, instances, memoryMiB uint32, name string) nvml.GpuInstanceProfileInfo_v2 {
	var info nvml.GpuInstanceProfileInfo_v2
	info.Id = id
	info.SliceCount = sliceCount
	info.InstanceCount = instances
	info.MemorySizeMB = uint64(memoryMiB)
	copy(info.Name[:], []byte(name))
	return info
}
