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

const maxNameLength = 253

// PhysicalGPUName builds a stable PhysicalGPU resource name.
func PhysicalGPUName(nodeName string, dev Device) string {
	index := dev.Index
	if index == "" {
		index = "0"
	}

	parts := []string{nodeName, index}
	if dev.VendorID != "" {
		parts = append(parts, strings.ToLower(dev.VendorID))
	}
	if dev.DeviceID != "" {
		parts = append(parts, strings.ToLower(dev.DeviceID))
	}

	name := sanitizeName(strings.Join(parts, "-"))
	if name == "" {
		return "pgpu"
	}

	if len(name) > maxNameLength {
		name = truncateName(name, maxNameLength)
		if name == "" {
			return "pgpu"
		}
	}

	return name
}

func sanitizeName(name string) string {
	return normalizeLabelValue(name)
}

func truncateName(name string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(name) <= limit {
		return name
	}
	name = name[:limit]
	return strings.Trim(name, "-")
}
