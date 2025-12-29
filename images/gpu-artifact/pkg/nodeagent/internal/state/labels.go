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

package state

import "strings"

// LabelsForDevice returns the labels applied to a PhysicalGPU object.
func LabelsForDevice(nodeName string, dev Device) map[string]string {
	labels := map[string]string{
		LabelNode: nodeName,
	}

	if vendor := VendorLabel(dev); vendor != "" {
		labels[LabelVendor] = vendor
	}
	if device := DeviceLabel(dev.DeviceName); device != "" {
		labels[LabelDevice] = device
	}

	return labels
}

// VendorLabel returns a normalized vendor label value.
func VendorLabel(dev Device) string {
	switch strings.ToLower(dev.VendorID) {
	case "10de":
		return "nvidia"
	case "1002":
		return "amd"
	case "8086":
		return "intel"
	}

	name := strings.ToLower(dev.VendorName)
	switch {
	case strings.Contains(name, "nvidia"):
		return "nvidia"
	case strings.Contains(name, "advanced micro devices"), strings.Contains(name, "amd"):
		return "amd"
	case strings.Contains(name, "intel"):
		return "intel"
	default:
		return ""
	}
}

// DeviceLabel returns a normalized device label value.
func DeviceLabel(deviceName string) string {
	deviceName = strings.TrimSpace(deviceName)
	if deviceName == "" {
		return ""
	}

	if start := strings.Index(deviceName, "["); start >= 0 {
		if end := strings.Index(deviceName[start+1:], "]"); end >= 0 {
			candidate := strings.TrimSpace(deviceName[start+1 : start+1+end])
			if candidate != "" {
				deviceName = candidate
			}
		}
	}

	return normalizeLabelValue(deviceName)
}

func normalizeLabelValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))

	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	normalized := strings.Trim(builder.String(), "-")
	if normalized == "" {
		return ""
	}
	return normalized
}
