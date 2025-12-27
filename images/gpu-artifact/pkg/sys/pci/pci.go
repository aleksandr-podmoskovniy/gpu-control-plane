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

package pci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultHostSysRoot = "/host-sys"
	defaultSysRoot     = "/sys"
)

// Device describes a PCI device read from sysfs.
type Device struct {
	Address    string
	ClassCode  string
	VendorID   string
	DeviceID   string
	DriverName string
}

// Reader lists PCI devices.
type Reader interface {
	List(ctx context.Context) ([]Device, error)
}

// SysfsReader reads PCI devices from sysfs.
type SysfsReader struct {
	SysRoot string
}

// NewSysfsReader creates a sysfs-based PCI reader.
func NewSysfsReader(sysRoot string) *SysfsReader {
	return &SysfsReader{SysRoot: sysRoot}
}

// List returns PCI devices from sysfs without filtering by vendor or class.
func (r *SysfsReader) List(ctx context.Context) ([]Device, error) {
	root := r.SysRoot
	if root == "" {
		root = defaultHostSysRoot
	}
	base := filepath.Join(root, "bus/pci/devices")
	if _, err := os.Stat(base); err != nil {
		base = filepath.Join(defaultSysRoot, "bus/pci/devices")
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("read pci devices: %w", err)
	}

	devices := make([]Device, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		addr := entry.Name()
		devicePath := filepath.Join(base, addr)

		classRaw, err := readTrim(filepath.Join(devicePath, "class"))
		if err != nil {
			continue
		}
		classCode := normalizeClassCode(classRaw)
		if classCode == "" {
			continue
		}

		vendorRaw, err := readTrim(filepath.Join(devicePath, "vendor"))
		if err != nil {
			continue
		}
		deviceRaw, err := readTrim(filepath.Join(devicePath, "device"))
		if err != nil {
			continue
		}

		dev := Device{
			Address:   addr,
			ClassCode: classCode,
			VendorID:  normalizeHexID(vendorRaw),
			DeviceID:  normalizeHexID(deviceRaw),
		}
		if driverName := readDriverName(devicePath); driverName != "" {
			dev.DriverName = driverName
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

func normalizeHexID(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	return value
}

func normalizeClassCode(raw string) string {
	value := normalizeHexID(raw)
	if len(value) < 4 {
		return ""
	}
	return value[:4]
}

func readTrim(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readDriverName(devicePath string) string {
	link, err := os.Readlink(filepath.Join(devicePath, "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}
