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
	"path/filepath"
	"strconv"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

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
