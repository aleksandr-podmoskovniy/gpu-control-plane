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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

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
