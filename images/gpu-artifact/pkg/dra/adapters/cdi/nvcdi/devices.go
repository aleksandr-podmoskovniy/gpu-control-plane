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
		if strings.EqualFold(deviceType, "mig") {
			return nil, nil, fmt.Errorf("MIG device %q requires a real MIG instance", dev.Device)
		}
		uuid := attrString(dev.Attributes, allocatable.AttrGPUUUID)
		if uuid == "" {
			return nil, nil, fmt.Errorf("GPU UUID is missing for device %q", dev.Device)
		}

		specs, err := w.nvcdi.GetDeviceSpecsByID(uuid)
		if err != nil {
			return nil, nil, fmt.Errorf("get CDI spec for %q: %w", dev.Device, err)
		}
		if len(specs) == 0 {
			return nil, nil, fmt.Errorf("empty CDI spec for %q", dev.Device)
		}

		name := claimDeviceName(req.ClaimUID, dev.Device)
		specs[0].Name = name
		deviceSpecs = append(deviceSpecs, specs[0])
		deviceIDs[dev.Device] = []string{cdiparser.QualifiedName(w.vendor, w.class, name)}
	}
	return deviceSpecs, deviceIDs, nil
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
