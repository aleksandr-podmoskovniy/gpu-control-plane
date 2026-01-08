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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Unprepare restores the original driver.
func (m *Manager) Unprepare(_ context.Context, state domain.PreparedVfioDevice) error {
	if m == nil {
		return errors.New("vfio manager is not configured")
	}
	if state.PCIBusID == "" {
		return errors.New("pci bus id is required")
	}
	target := state.OriginalDriver
	if target == "" {
		target = defaultDriver
	}
	if target == vfioDriver {
		return nil
	}
	current, err := m.readDriver(state.PCIBusID)
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}
	if err := m.bindToDriver(state.PCIBusID, current, target); err != nil {
		return err
	}
	return nil
}
