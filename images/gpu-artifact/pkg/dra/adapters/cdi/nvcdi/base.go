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

package nvcdi

import (
	"context"
	"errors"
	"fmt"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

const baseSpecIdentifier = "base"

// BaseDevice describes a device entry to publish into the base CDI spec.
type BaseDevice struct {
	Name string
	UUID string
}

// WriteBase writes the base CDI spec containing device entries.
func (w *Writer) WriteBase(_ context.Context, devices []BaseDevice) error {
	if w == nil {
		return errors.New("CDI writer is nil")
	}
	if len(devices) == 0 {
		return nil
	}

	if err := w.initNVML(); err != nil {
		return err
	}
	defer w.nvml.Shutdown()

	commonEdits, err := w.commonEdits(w.nvcdiDevice)
	if err != nil {
		return err
	}

	deviceSpecs, err := w.buildBaseDeviceSpecs(devices)
	if err != nil {
		return err
	}
	if len(deviceSpecs) == 0 {
		return nil
	}

	spec, err := buildSpec(w.vendor, w.deviceClass, deviceSpecs, commonEdits.ContainerEdits)
	if err != nil {
		return err
	}

	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.deviceClass, baseSpecIdentifier)
	return w.writeSpec(spec, specName)
}

func (w *Writer) buildBaseDeviceSpecs(devices []BaseDevice) ([]cdispec.Device, error) {
	deviceSpecs := make([]cdispec.Device, 0, len(devices))
	for _, dev := range devices {
		if dev.Name == "" || dev.UUID == "" {
			continue
		}
		specs, err := w.nvcdiDevice.GetDeviceSpecsByID(dev.UUID)
		if err != nil {
			return nil, fmt.Errorf("get CDI spec for %q: %w", dev.Name, err)
		}
		if len(specs) == 0 {
			return nil, fmt.Errorf("empty CDI spec for %q", dev.Name)
		}
		specs[0].Name = dev.Name
		deviceSpecs = append(deviceSpecs, specs[0])
	}
	return deviceSpecs, nil
}
