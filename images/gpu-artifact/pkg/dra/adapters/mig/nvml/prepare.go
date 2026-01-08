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

// Prepare creates a MIG instance for the requested placement.
func (m *Manager) Prepare(_ context.Context, req domain.MigPrepareRequest) (domain.PreparedMigDevice, error) {
	if m == nil || m.lib == nil {
		return domain.PreparedMigDevice{}, errors.New("nvml is not configured")
	}
	if err := validatePrepareRequest(req); err != nil {
		return domain.PreparedMigDevice{}, err
	}

	if err := initNVML(m.lib); err != nil {
		return domain.PreparedMigDevice{}, err
	}
	defer m.lib.Shutdown()

	device, ret := m.lib.DeviceGetHandleByPciBusId(req.PCIBusID)
	if ret != nvml.SUCCESS {
		return domain.PreparedMigDevice{}, fmt.Errorf("get device by pci %q: %s", req.PCIBusID, nvml.ErrorString(ret))
	}

	if err := ensureMigMode(m.lib, device); err != nil {
		return domain.PreparedMigDevice{}, err
	}

	profileInfo, err := lookupProfile(device, req.ProfileID)
	if err != nil {
		return domain.PreparedMigDevice{}, err
	}

	existing, ok := m.findExisting(device, profileInfo, req)
	if ok {
		return existing, nil
	}

	return createMigDevice(m.lib, device, profileInfo, req)
}
