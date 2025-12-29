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
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

var (
	// ErrNVMLUnavailable reports NVML init/device access failures.
	ErrNVMLUnavailable = errors.New("nvml unavailable")
	// ErrNVMLQueryFailed reports NVML query failures.
	ErrNVMLQueryFailed = errors.New("nvml query failed")
	// ErrMissingPCIAddress reports missing PCI address.
	ErrMissingPCIAddress = errors.New("missing pci address")
)

// DeviceSnapshot contains NVML capabilities and current state.
type DeviceSnapshot struct {
	Capabilities *gpuv1alpha1.GPUCapabilities
	CurrentState *gpuv1alpha1.GPUCurrentState
}

// NVMLReader opens a session and reads NVML data.
type NVMLReader struct {
	nvml NVML
}

// NVMLSession reads devices using an initialized NVML instance.
type NVMLSession struct {
	nvml          NVML
	driverVersion string
	cudaVersion   string
}

// NewNVMLReader constructs a reader for NVML.
func NewNVMLReader(nvmlService NVML) *NVMLReader {
	return &NVMLReader{nvml: nvmlService}
}

// Open initializes NVML and returns a session.
func (r *NVMLReader) Open() (CapabilitiesSession, error) {
	if r == nil || r.nvml == nil {
		return nil, newReadError(ErrNVMLUnavailable, "NVML is not configured")
	}

	ret := r.nvml.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return nil, newReadError(ErrNVMLUnavailable, "NVML init failed: %s", r.nvml.ErrorString(ret))
	}

	driverVersion, ret := r.nvml.SystemGetDriverVersion()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML driver version failed: %s", r.nvml.ErrorString(ret))
	}

	cudaRaw, ret := r.nvml.SystemGetCudaDriverVersion()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML CUDA version failed: %s", r.nvml.ErrorString(ret))
	}

	return &NVMLSession{
		nvml:          r.nvml,
		driverVersion: driverVersion,
		cudaVersion:   formatCudaVersion(cudaRaw),
	}, nil
}

// Close shuts down NVML for this session.
func (s *NVMLSession) Close() {
	if s == nil || s.nvml == nil {
		return
	}
	_ = s.nvml.Shutdown()
}

// ReadDevice returns NVML capabilities and current state for a PCI address.
func (s *NVMLSession) ReadDevice(pciAddress string) (*DeviceSnapshot, error) {
	if pciAddress == "" {
		return nil, newReadError(ErrMissingPCIAddress, "pci address is empty")
	}
	if s == nil || s.nvml == nil {
		return nil, newReadError(ErrNVMLUnavailable, "NVML is not initialized")
	}

	dev, ret := s.nvml.DeviceByPCI(pciAddress)
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLUnavailable, "NVML device lookup failed: %s", s.nvml.ErrorString(ret))
	}

	name, ret := dev.GetName()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML device name failed: %s", s.nvml.ErrorString(ret))
	}

	mem, ret := dev.GetMemoryInfo()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML memory info failed: %s", s.nvml.ErrorString(ret))
	}

	major, minor, ret := dev.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML compute capability failed: %s", s.nvml.ErrorString(ret))
	}

	arch, ret := dev.GetArchitecture()
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLQueryFailed, "NVML architecture failed: %s", s.nvml.ErrorString(ret))
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

	current := &gpuv1alpha1.GPUCurrentState{
		Nvidia: &gpuv1alpha1.NvidiaCurrentState{
			GPUUUID:             nvmlString(dev.GetUUID),
			DriverVersion:       s.driverVersion,
			CUDAVersion:         s.cudaVersion,
			PowerLimitCurrentW:  nvmlPowerLimit(dev.GetPowerManagementLimit),
			PowerLimitEnforcedW: nvmlPowerLimit(dev.GetEnforcedPowerLimit),
		},
	}

	if mode := readMigMode(dev); mode != nil {
		current.Nvidia.MIG = mode
	}

	return &DeviceSnapshot{
		Capabilities: capabilities,
		CurrentState: current,
	}, nil
}

type nvmlReadError struct {
	reason  error
	message string
}

func (e *nvmlReadError) Error() string {
	return e.message
}

func (e *nvmlReadError) Unwrap() error {
	return e.reason
}

func newReadError(reason error, format string, args ...interface{}) error {
	return &nvmlReadError{
		reason:  reason,
		message: fmt.Sprintf(format, args...),
	}
}

func readMigMode(dev NVMLDevice) *gpuv1alpha1.NvidiaMIGState {
	current, _, ret := dev.GetMigMode()
	if ret != nvml.SUCCESS {
		return nil
	}
	return &gpuv1alpha1.NvidiaMIGState{Mode: migModeFromNVML(current)}
}

func readMigCapabilities(dev NVMLDevice) (*bool, *gpuv1alpha1.NvidiaMIGCapabilities) {
	_, _, ret := dev.GetMigMode()
	switch ret {
	case nvml.SUCCESS:
		supported := true
		return boolPtr(supported), buildMigProfiles(dev)
	case nvml.ERROR_NOT_SUPPORTED:
		supported := false
		return boolPtr(supported), nil
	default:
		return nil, nil
	}
}

func buildMigProfiles(dev NVMLDevice) *gpuv1alpha1.NvidiaMIGCapabilities {
	var profiles []gpuv1alpha1.NvidiaMIGProfile
	var totalSlices int32

	for profile := 0; profile < nvml.GPU_INSTANCE_PROFILE_COUNT; profile++ {
		info, ret := dev.GetGpuInstanceProfileInfoV3(profile)
		if ret != nvml.SUCCESS {
			continue
		}
		name := nvmlName(info.Name[:])
		if name == "" {
			continue
		}
		profiles = append(profiles, gpuv1alpha1.NvidiaMIGProfile{
			ProfileID:    int32(info.Id),
			Name:         name,
			MemoryMiB:    int32(info.MemorySizeMB),
			SliceCount:   int32(info.SliceCount),
			MaxInstances: int32(info.InstanceCount),
		})
		if int32(info.SliceCount) > totalSlices {
			totalSlices = int32(info.SliceCount)
		}
	}

	if len(profiles) == 0 {
		return nil
	}
	return &gpuv1alpha1.NvidiaMIGCapabilities{
		TotalSlices: totalSlices,
		Profiles:    profiles,
	}
}

func formatCudaVersion(raw int) string {
	if raw <= 0 {
		return ""
	}
	major := raw / 1000
	minor := (raw % 1000) / 10
	return fmt.Sprintf("%d.%d", major, minor)
}

func architectureName(arch nvml.DeviceArchitecture) string {
	switch arch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "Kepler"
	case nvml.DEVICE_ARCH_MAXWELL:
		return "Maxwell"
	case nvml.DEVICE_ARCH_PASCAL:
		return "Pascal"
	case nvml.DEVICE_ARCH_VOLTA:
		return "Volta"
	case nvml.DEVICE_ARCH_TURING:
		return "Turing"
	case nvml.DEVICE_ARCH_AMPERE:
		return "Ampere"
	case nvml.DEVICE_ARCH_ADA:
		return "Ada"
	case nvml.DEVICE_ARCH_HOPPER:
		return "Hopper"
	case nvml.DEVICE_ARCH_BLACKWELL:
		return "Blackwell"
	default:
		return ""
	}
}

func migModeFromNVML(mode int) gpuv1alpha1.MIGModeState {
	switch mode {
	case nvml.DEVICE_MIG_ENABLE:
		return gpuv1alpha1.MIGModeEnabled
	case nvml.DEVICE_MIG_DISABLE:
		return gpuv1alpha1.MIGModeDisabled
	default:
		return gpuv1alpha1.MIGModeUnknown
	}
}

func nvmlName(raw []uint8) string {
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i])
		}
	}
	return string(raw)
}

func nvmlString(call func() (string, nvml.Return)) string {
	value, ret := call()
	if ret != nvml.SUCCESS {
		return ""
	}
	return value
}

func nvmlPowerLimit(call func() (uint32, nvml.Return)) *int64 {
	value, ret := call()
	if ret != nvml.SUCCESS {
		return nil
	}
	limit := milliwattsToWatts(value)
	return &limit
}

func milliwattsToWatts(value uint32) int64 {
	return int64(value) / 1000
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
