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
	"os"
	"path/filepath"
	"strconv"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

const (
	defaultSysfsRoot = "/sys/bus/pci/devices"
	driversRoot      = "/sys/bus/pci/drivers"
	vfioDriver       = "vfio-pci"
	defaultDriver    = "nvidia"
)

// Manager binds devices to vfio-pci.
type Manager struct {
	sysfsRoot string
}

// Options configure the VFIO manager.
type Options struct {
	SysfsRoot string
}

// New constructs a VFIO manager.
func New(opts Options) *Manager {
	root := opts.SysfsRoot
	if root == "" {
		root = defaultSysfsRoot
	}
	return &Manager{sysfsRoot: root}
}

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

func (m *Manager) bindToDriver(pciBusID, fromDriver, toDriver string) error {
	if err := m.ensureDriver(toDriver); err != nil {
		return err
	}
	if err := m.writeDriverOverride(pciBusID, toDriver); err != nil {
		return err
	}
	if err := m.unbind(pciBusID, fromDriver); err != nil {
		return err
	}
	if err := m.bind(pciBusID, toDriver); err != nil {
		return err
	}
	if err := m.clearDriverOverride(pciBusID); err != nil {
		return err
	}
	return nil
}

func (m *Manager) devicePath(pciBusID string) string {
	return filepath.Join(m.sysfsRoot, pciBusID)
}

func (m *Manager) readDriver(pciBusID string) (string, error) {
	path := filepath.Join(m.devicePath(pciBusID), "driver")
	link, err := os.Readlink(path)
	if err != nil {
		return "", fmt.Errorf("read driver for %s: %w", pciBusID, err)
	}
	return filepath.Base(link), nil
}

func (m *Manager) readIommuGroup(pciBusID string) (int, error) {
	path := filepath.Join(m.devicePath(pciBusID), "iommu_group")
	link, err := os.Readlink(path)
	if err != nil {
		return 0, fmt.Errorf("read iommu group for %s: %w", pciBusID, err)
	}
	group := filepath.Base(link)
	id, err := strconv.Atoi(group)
	if err != nil {
		return 0, fmt.Errorf("parse iommu group for %s: %w", pciBusID, err)
	}
	return id, nil
}

func (m *Manager) ensureDriver(driver string) error {
	path := filepath.Join(driversRoot, driver)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("driver %q is not available: %w", driver, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("driver %q path is not a directory", driver)
	}
	return nil
}

func (m *Manager) writeDriverOverride(pciBusID, driver string) error {
	path := filepath.Join(m.devicePath(pciBusID), "driver_override")
	if err := os.WriteFile(path, []byte(driver), 0o644); err != nil {
		return fmt.Errorf("write driver override for %s: %w", pciBusID, err)
	}
	return nil
}

func (m *Manager) clearDriverOverride(pciBusID string) error {
	path := filepath.Join(m.devicePath(pciBusID), "driver_override")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		return fmt.Errorf("clear driver override for %s: %w", pciBusID, err)
	}
	return nil
}

func (m *Manager) unbind(pciBusID, driver string) error {
	path := filepath.Join(driversRoot, driver, "unbind")
	if err := os.WriteFile(path, []byte(pciBusID), 0o644); err != nil {
		return fmt.Errorf("unbind %s from %s: %w", pciBusID, driver, err)
	}
	return nil
}

func (m *Manager) bind(pciBusID, driver string) error {
	path := filepath.Join(driversRoot, driver, "bind")
	if err := os.WriteFile(path, []byte(pciBusID), 0o644); err != nil {
		return fmt.Errorf("bind %s to %s: %w", pciBusID, driver, err)
	}
	return nil
}
