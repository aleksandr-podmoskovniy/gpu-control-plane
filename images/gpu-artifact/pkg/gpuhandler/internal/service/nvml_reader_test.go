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

package service

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
	name          string
	uuid          string
	memory        nvml.Memory
	major         int
	minor         int
	arch          nvml.DeviceArchitecture
	boardPart     string
	powerLimit    uint32
	enforcedPower uint32
	minPower      uint32
	maxPower      uint32
	migMode       int
	migRet        nvml.Return
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
	return d.boardPart, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetPowerManagementLimit() (uint32, nvml.Return) {
	return d.powerLimit, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetEnforcedPowerLimit() (uint32, nvml.Return) {
	return d.enforcedPower, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetPowerManagementLimitConstraints() (uint32, uint32, nvml.Return) {
	return d.minPower, d.maxPower, nvml.SUCCESS
}

func (d *fakeNVMLDevice) GetMigMode() (int, int, nvml.Return) {
	return d.migMode, 0, d.migRet
}

func (d *fakeNVMLDevice) GetGpuInstanceProfileInfoV3(_ int) (nvml.GpuInstanceProfileInfo_v3, nvml.Return) {
	return nvml.GpuInstanceProfileInfo_v3{}, nvml.ERROR_NOT_SUPPORTED
}
