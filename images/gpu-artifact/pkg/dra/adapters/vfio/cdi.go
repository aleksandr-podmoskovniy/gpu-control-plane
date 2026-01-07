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
	"strings"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

const (
	vfioRoot = "/dev/vfio"
)

// CDIWriter generates CDI specs for vfio-pci devices.
type CDIWriter struct {
	vendor    string
	class     string
	cdiRoot   string
	sysfsRoot string
	cache     *cdiapi.Cache
}

// CDIOptions configure the VFIO CDI writer.
type CDIOptions struct {
	Vendor    string
	Class     string
	CDIRoot   string
	SysfsRoot string
}

// NewCDIWriter constructs a VFIO CDI writer.
func NewCDIWriter(opts CDIOptions) (*CDIWriter, error) {
	vendor := opts.Vendor
	if vendor == "" {
		return nil, errors.New("vendor is required")
	}
	class := opts.Class
	if class == "" {
		return nil, errors.New("class is required")
	}
	cdiRoot := opts.CDIRoot
	if cdiRoot == "" {
		cdiRoot = "/etc/cdi"
	}
	sysfsRoot := opts.SysfsRoot
	if sysfsRoot == "" {
		sysfsRoot = defaultSysfsRoot
	}
	if err := os.MkdirAll(cdiRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create CDI root %q: %w", cdiRoot, err)
	}
	cache, err := cdiapi.NewCache(cdiapi.WithSpecDirs(cdiRoot))
	if err != nil {
		return nil, fmt.Errorf("create CDI cache: %w", err)
	}
	return &CDIWriter{
		vendor:    vendor,
		class:     class,
		cdiRoot:   cdiRoot,
		sysfsRoot: sysfsRoot,
		cache:     cache,
	}, nil
}

// Write generates CDI specs for vfio-pci devices.
func (w *CDIWriter) Write(_ context.Context, req domain.PrepareRequest) (map[string][]string, error) {
	if w == nil {
		return nil, errors.New("vfio CDI writer is nil")
	}
	if req.ClaimUID == "" {
		return nil, errors.New("claim UID is required")
	}
	if len(req.Devices) == 0 {
		return nil, errors.New("no devices to prepare")
	}

	deviceSpecs := make([]cdispec.Device, 0, len(req.Devices))
	deviceIDs := make(map[string][]string, len(req.Devices))
	for _, dev := range req.Devices {
		pci := attrString(dev.Attributes, allocatable.AttrPCIAddress)
		if pci == "" {
			return nil, fmt.Errorf("pci address is missing for device %q", dev.Device)
		}
		group, err := w.readIommuGroup(pci)
		if err != nil {
			return nil, err
		}
		name := claimDeviceName(req.ClaimUID, dev.Device)
		deviceSpecs = append(deviceSpecs, cdispec.Device{
			Name: name,
			ContainerEdits: cdispec.ContainerEdits{
				DeviceNodes: []*cdispec.DeviceNode{
					{Path: filepath.Join(vfioRoot, strconv.Itoa(group))},
				},
			},
		})
		deviceIDs[dev.Device] = []string{cdiparser.QualifiedName(w.vendor, w.class, name)}
	}

	spec := cdispec.Spec{
		Kind: fmt.Sprintf("%s/%s", w.vendor, w.class),
		ContainerEdits: cdispec.ContainerEdits{
			DeviceNodes: []*cdispec.DeviceNode{{Path: filepath.Join(vfioRoot, "vfio")}},
		},
		Devices: deviceSpecs,
	}
	minVersion, err := cdispec.MinimumRequiredVersion(&spec)
	if err != nil {
		return nil, fmt.Errorf("detect CDI spec version: %w", err)
	}
	spec.Version = minVersion

	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.class, req.ClaimUID)
	if err := w.cache.WriteSpec(&spec, specName); err != nil {
		return nil, fmt.Errorf("write CDI spec %q: %w", specName, err)
	}

	return deviceIDs, nil
}

// Delete removes CDI specs for a claim.
func (w *CDIWriter) Delete(_ context.Context, claimUID string) error {
	if w == nil {
		return errors.New("vfio CDI writer is nil")
	}
	if claimUID == "" {
		return errors.New("claim UID is required")
	}
	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.class, claimUID)
	return w.cache.RemoveSpec(specName)
}

func (w *CDIWriter) readIommuGroup(pciBusID string) (int, error) {
	path := filepath.Join(w.sysfsRoot, pciBusID, "iommu_group")
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

func claimDeviceName(claimUID, deviceName string) string {
	return fmt.Sprintf("%s-%s", claimUID, deviceName)
}

func attrString(attrs map[string]allocatable.AttributeValue, key string) string {
	if attrs == nil {
		return ""
	}
	val, ok := attrs[key]
	if !ok || val.String == nil {
		return ""
	}
	return strings.TrimSpace(*val.String)
}
