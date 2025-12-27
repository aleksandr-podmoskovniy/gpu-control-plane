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
	"sort"
	"strconv"
	"strings"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/pci"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/pciids"
)

const (
	nvidiaVendorID = "10de"
)

// PCIProvider lists GPU-like PCI devices on a node.
type PCIProvider interface {
	Scan(ctx context.Context) ([]state.Device, error)
}

// SysfsPCIProvider scans sysfs for PCI devices.
type SysfsPCIProvider struct {
	SysRoot  string
	Resolver *pciids.Resolver
	Reader   pci.Reader
}

// NewSysfsPCIProvider creates a sysfs-based PCI provider.
func NewSysfsPCIProvider(sysRoot string, resolver *pciids.Resolver) *SysfsPCIProvider {
	return &SysfsPCIProvider{
		SysRoot:  sysRoot,
		Resolver: resolver,
		Reader:   pci.NewSysfsReader(sysRoot),
	}
}

// Scan returns detected GPU-like PCI devices.
func (p *SysfsPCIProvider) Scan(ctx context.Context) ([]state.Device, error) {
	reader := p.Reader
	if reader == nil {
		reader = pci.NewSysfsReader(p.SysRoot)
	}
	rawDevices, err := reader.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pci devices: %w", err)
	}

	devices := make([]state.Device, 0, len(rawDevices))
	for _, raw := range rawDevices {
		if !isGPUClass(raw.ClassCode) {
			continue
		}
		// v0: only NVIDIA devices. TODO: add AMD/Intel when supported.
		if raw.VendorID != nvidiaVendorID {
			continue
		}

		device := state.Device{
			Address:    raw.Address,
			ClassCode:  raw.ClassCode,
			VendorID:   raw.VendorID,
			DeviceID:   raw.DeviceID,
			DriverName: raw.DriverName,
		}

		if p.Resolver != nil {
			device.ClassName = p.Resolver.ClassName(device.ClassCode)
			device.VendorName = p.Resolver.VendorName(device.VendorID)
			device.DeviceName = p.Resolver.DeviceName(device.VendorID, device.DeviceID)
		}

		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Address < devices[j].Address
	})
	for i := range devices {
		devices[i].Index = strconv.Itoa(i)
	}

	return devices, nil
}

func isGPUClass(classCode string) bool {
	return strings.HasPrefix(strings.ToLower(classCode), "03")
}
