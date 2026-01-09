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
	"fmt"
	"strings"

	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func (w *Writer) buildDeviceSpecs(req domain.PrepareRequest) ([]cdispec.Device, map[string][]string, error) {
	deviceSpecs := make([]cdispec.Device, 0, len(req.Devices))
	deviceIDs := make(map[string][]string, len(req.Devices))
	for _, dev := range req.Devices {
		deviceType := attrString(dev.Attributes, allocatable.AttrDeviceType)
		uuid := attrString(dev.Attributes, allocatable.AttrGPUUUID)
		if strings.EqualFold(deviceType, "mig") {
			uuid = attrString(dev.Attributes, allocatable.AttrMigUUID)
			if uuid == "" {
				return nil, nil, fmt.Errorf("MIG uuid is missing for device %q", dev.Device)
			}
		}
		if uuid == "" {
			return nil, nil, fmt.Errorf("GPU UUID is missing for device %q", dev.Device)
		}

		specs, err := w.nvcdiClaim.GetDeviceSpecsByID(uuid)
		if err != nil {
			return nil, nil, fmt.Errorf("get CDI spec for %q: %w", dev.Device, err)
		}
		if len(specs) == 0 {
			return nil, nil, fmt.Errorf("empty CDI spec for %q", dev.Device)
		}

		name := claimDeviceName(req.ClaimUID, dev.Device)
		specs[0].Name = name
		if strings.EqualFold(deviceType, "mig") {
			nodes, err := w.migDeviceNodes(uuid)
			if err != nil {
				return nil, nil, fmt.Errorf("build MIG device nodes for %q: %w", dev.Device, err)
			}
			specs[0].ContainerEdits.DeviceNodes = append(specs[0].ContainerEdits.DeviceNodes, nodes...)
		}
		applyMpsEdits(&specs[0].ContainerEdits, dev.Attributes)
		deviceSpecs = append(deviceSpecs, specs[0])
		deviceIDs[dev.Device] = []string{cdiparser.QualifiedName(w.vendor, w.claimClass, name)}
	}
	return deviceSpecs, deviceIDs, nil
}

func claimDeviceName(claimUID, deviceName string) string {
	return fmt.Sprintf("%s-%s", claimUID, deviceName)
}
