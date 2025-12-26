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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/pciids"
)

const (
	defaultHostSysRoot = "/host-sys"
	defaultSysRoot     = "/sys"
)

// PCIProvider lists GPU-like PCI devices on a node.
type PCIProvider interface {
	Scan(ctx context.Context) ([]state.Device, error)
}

// SysfsPCIProvider scans sysfs for PCI devices.
type SysfsPCIProvider struct {
	SysRoot  string
	Resolver *pciids.Resolver
}

// NewSysfsPCIProvider creates a sysfs-based PCI provider.
func NewSysfsPCIProvider(sysRoot string, resolver *pciids.Resolver) *SysfsPCIProvider {
	return &SysfsPCIProvider{SysRoot: sysRoot, Resolver: resolver}
}

// Scan returns detected GPU-like PCI devices.
func (p *SysfsPCIProvider) Scan(_ context.Context) ([]state.Device, error) {
	root := p.SysRoot
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

	devices := make([]state.Device, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		addr := entry.Name()
		devicePath := filepath.Join(base, addr)

		classRaw, err := readTrim(filepath.Join(devicePath, "class"))
		if err != nil {
			continue
		}
		classCode := normalizeClassCode(classRaw)
		if classCode == "" || !isGPUClass(classCode) {
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

		device := state.Device{
			Address:   addr,
			ClassCode: classCode,
			VendorID:  normalizeHexID(vendorRaw),
			DeviceID:  normalizeHexID(deviceRaw),
		}

		if p.Resolver != nil {
			device.ClassName = p.Resolver.ClassName(device.ClassCode)
			device.VendorName = p.Resolver.VendorName(device.VendorID)
			device.DeviceName = p.Resolver.DeviceName(device.VendorID, device.DeviceID)
		}

		devices = append(devices, device)
	}

	return devices, nil
}

func isGPUClass(classCode string) bool {
	return strings.HasPrefix(strings.ToLower(classCode), "03")
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
