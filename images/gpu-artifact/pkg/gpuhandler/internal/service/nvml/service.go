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
