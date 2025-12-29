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
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// NVML exposes only the NVML operations needed by the gpu-handler.
type NVML interface {
	Init() nvml.Return
	Shutdown() nvml.Return
	SystemGetDriverVersion() (string, nvml.Return)
	SystemGetCudaDriverVersion() (int, nvml.Return)
	DeviceByPCI(pciBusID string) (NVMLDevice, nvml.Return)
	ErrorString(ret nvml.Return) string
}

// NVMLDevice is a thin wrapper over nvml.Device with only required methods.
type NVMLDevice interface {
	GetName() (string, nvml.Return)
	GetUUID() (string, nvml.Return)
	GetMemoryInfo() (nvml.Memory, nvml.Return)
	GetCudaComputeCapability() (int, int, nvml.Return)
	GetArchitecture() (nvml.DeviceArchitecture, nvml.Return)
	GetBoardPartNumber() (string, nvml.Return)
	GetPowerManagementLimit() (uint32, nvml.Return)
	GetEnforcedPowerLimit() (uint32, nvml.Return)
	GetPowerManagementLimitConstraints() (uint32, uint32, nvml.Return)
	GetMigMode() (int, int, nvml.Return)
	GetGpuInstanceProfileInfoV3(profile int) (nvml.GpuInstanceProfileInfo_v3, nvml.Return)
}

// NVMLService provides access to NVML through the go-nvml library.
type NVMLService struct {
	lib nvml.Interface
}

// NewNVML constructs an NVML service.
func NewNVML() *NVMLService {
	return &NVMLService{lib: nvml.New()}
}

// Init initializes NVML.
func (s *NVMLService) Init() nvml.Return {
	return s.lib.Init()
}

// Shutdown shuts down NVML.
func (s *NVMLService) Shutdown() nvml.Return {
	return s.lib.Shutdown()
}

// SystemGetDriverVersion returns the NVIDIA driver version.
func (s *NVMLService) SystemGetDriverVersion() (string, nvml.Return) {
	return s.lib.SystemGetDriverVersion()
}

// SystemGetCudaDriverVersion returns the CUDA driver version number.
func (s *NVMLService) SystemGetCudaDriverVersion() (int, nvml.Return) {
	return s.lib.SystemGetCudaDriverVersion()
}

// ErrorString formats NVML return codes.
func (s *NVMLService) ErrorString(ret nvml.Return) string {
	return s.lib.ErrorString(ret)
}

// DeviceByPCI returns an NVML device handle for a PCI bus ID.
func (s *NVMLService) DeviceByPCI(pciBusID string) (NVMLDevice, nvml.Return) {
	normalized := normalizePCIBusID(pciBusID)
	dev, ret := s.lib.DeviceGetHandleByPciBusId(normalized)
	if ret != nvml.SUCCESS {
		return nil, ret
	}
	return nvmlDevice{device: dev}, ret
}

type nvmlDevice struct {
	device nvml.Device
}

func (d nvmlDevice) GetName() (string, nvml.Return) {
	return d.device.GetName()
}

func (d nvmlDevice) GetUUID() (string, nvml.Return) {
	return d.device.GetUUID()
}

func (d nvmlDevice) GetMemoryInfo() (nvml.Memory, nvml.Return) {
	return d.device.GetMemoryInfo()
}

func (d nvmlDevice) GetCudaComputeCapability() (int, int, nvml.Return) {
	return d.device.GetCudaComputeCapability()
}

func (d nvmlDevice) GetArchitecture() (nvml.DeviceArchitecture, nvml.Return) {
	return d.device.GetArchitecture()
}

func (d nvmlDevice) GetBoardPartNumber() (string, nvml.Return) {
	return d.device.GetBoardPartNumber()
}

func (d nvmlDevice) GetPowerManagementLimit() (uint32, nvml.Return) {
	return d.device.GetPowerManagementLimit()
}

func (d nvmlDevice) GetEnforcedPowerLimit() (uint32, nvml.Return) {
	return d.device.GetEnforcedPowerLimit()
}

func (d nvmlDevice) GetPowerManagementLimitConstraints() (uint32, uint32, nvml.Return) {
	return d.device.GetPowerManagementLimitConstraints()
}

func (d nvmlDevice) GetMigMode() (int, int, nvml.Return) {
	return d.device.GetMigMode()
}

func (d nvmlDevice) GetGpuInstanceProfileInfoV3(profile int) (nvml.GpuInstanceProfileInfo_v3, nvml.Return) {
	return d.device.GetGpuInstanceProfileInfoV(profile).V3()
}

func normalizePCIBusID(busID string) string {
	busID = strings.TrimSpace(busID)
	parts := strings.Split(busID, ":")
	if len(parts) != 3 {
		return busID
	}
	if len(parts[0]) < 8 {
		if value, err := strconv.ParseUint(parts[0], 16, 32); err == nil {
			parts[0] = fmt.Sprintf("%08x", value)
		}
	}
	return strings.Join(parts, ":")
}
