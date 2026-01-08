//go:build linux
// +build linux

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

package vfio

import (
	"context"
	"errors"
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Prepare binds the device to vfio-pci and returns the original driver.
func (m *Manager) Prepare(_ context.Context, req domain.VfioPrepareRequest) (domain.PreparedVfioDevice, error) {
	if m == nil {
		return domain.PreparedVfioDevice{}, errors.New("vfio manager is not configured")
	}
	if req.PCIBusID == "" {
		return domain.PreparedVfioDevice{}, errors.New("pci bus id is required")
	}
	driver, err := m.readDriver(req.PCIBusID)
	if err != nil {
		return domain.PreparedVfioDevice{}, err
	}
	group, err := m.readIommuGroup(req.PCIBusID)
	if err != nil {
		return domain.PreparedVfioDevice{}, err
	}
	if driver == vfioDriver {
		return domain.PreparedVfioDevice{PCIBusID: req.PCIBusID, OriginalDriver: vfioDriver, IommuGroup: group}, nil
	}
	if driver != defaultDriver {
		return domain.PreparedVfioDevice{}, fmt.Errorf("unexpected driver %q for %s", driver, req.PCIBusID)
	}
	if err := m.bindToDriver(req.PCIBusID, driver, vfioDriver); err != nil {
		return domain.PreparedVfioDevice{}, err
	}
	return domain.PreparedVfioDevice{PCIBusID: req.PCIBusID, OriginalDriver: driver, IommuGroup: group}, nil
}
