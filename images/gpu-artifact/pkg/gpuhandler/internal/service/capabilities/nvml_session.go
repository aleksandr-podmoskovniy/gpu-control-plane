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

import "github.com/NVIDIA/go-nvml/pkg/nvml"

// NVMLSession reads devices using an initialized NVML instance.
type NVMLSession struct {
	nvml          NVML
	driverVersion string
	cudaVersion   string
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

	capabilities, err := buildCapabilities(dev)
	if err != nil {
		return nil, err
	}

	current, err := buildCurrentState(dev, s.driverVersion, s.cudaVersion)
	if err != nil {
		return nil, err
	}

	return &DeviceSnapshot{
		Capabilities: capabilities,
		CurrentState: current,
	}, nil
}
