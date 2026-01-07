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

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	nvmlapi "github.com/NVIDIA/go-nvml/pkg/nvml"
)

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

// NVMLService provides access to NVML through the go-nvml library.
type NVMLService struct {
	lib nvmlapi.Interface
}

// NewNVML constructs an NVML service.
func NewNVML() *NVMLService {
	return &NVMLService{lib: nvmlapi.New(nvmlLibraryOptions()...)}
}

// Init initializes NVML.
func (s *NVMLService) Init() nvmlapi.Return {
	return s.lib.Init()
}

// Shutdown shuts down NVML.
func (s *NVMLService) Shutdown() nvmlapi.Return {
	return s.lib.Shutdown()
}

// SystemGetDriverVersion returns the NVIDIA driver version.
func (s *NVMLService) SystemGetDriverVersion() (string, nvmlapi.Return) {
	return s.lib.SystemGetDriverVersion()
}

// SystemGetCudaDriverVersion returns the CUDA driver version number.
func (s *NVMLService) SystemGetCudaDriverVersion() (int, nvmlapi.Return) {
	return s.lib.SystemGetCudaDriverVersion()
}

// ErrorString formats NVML return codes.
func (s *NVMLService) ErrorString(ret nvmlapi.Return) string {
	return s.lib.ErrorString(ret)
}

// DeviceByPCI returns an NVML device handle for a PCI bus ID.
func (s *NVMLService) DeviceByPCI(pciBusID string) (NVMLDevice, nvmlapi.Return) {
	normalized := normalizePCIBusID(pciBusID)
	dev, ret := s.lib.DeviceGetHandleByPciBusId(normalized)
	if ret != nvmlapi.SUCCESS {
		return nil, ret
	}
	return nvmlDevice{device: dev}, ret
}

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

const nvmlLibraryName = "libnvidia-ml.so.1"

var nvmlLibrarySearchPaths = []string{
	"/usr/lib64",
	"/usr/lib/x86_64-linux-gnu",
	"/usr/lib/aarch64-linux-gnu",
	"/lib64",
	"/lib/x86_64-linux-gnu",
	"/lib/aarch64-linux-gnu",
}

func nvmlLibraryOptions() []nvmlapi.LibraryOption {
	driverRoot := strings.TrimSpace(os.Getenv("NVIDIA_DRIVER_ROOT"))
	if driverRoot == "" {
		return nil
	}
	path, err := findNVMLLibrary(driverRoot)
	if err == nil {
		return []nvmlapi.LibraryOption{nvmlapi.WithLibraryPath(path)}
	}
	return nil
}

func findNVMLLibrary(root string) (string, error) {
	for _, dir := range nvmlLibrarySearchPaths {
		candidate := filepath.Join(root, dir, nvmlLibraryName)
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			continue
		}
		return resolved, nil
	}
	return "", fmt.Errorf("nvml library %q not found under %s", nvmlLibraryName, root)
}
