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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

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
