// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package state

import (
	"sort"
	"strconv"
	"strings"
)

func extractDeviceSnapshots(labels map[string]string) []deviceSnapshot {
	devices := make(map[string]deviceSnapshot)
	for key, value := range labels {
		if !strings.HasPrefix(key, deviceLabelPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(key, deviceLabelPrefix)
		parts := strings.SplitN(suffix, ".", 2)
		if len(parts) != 2 {
			continue
		}
		index := canonicalIndex(parts[0])
		field := parts[1]

		info := devices[index]
		info.Index = index

		switch field {
		case "vendor":
			info.Vendor = strings.ToLower(value)
		case "device":
			info.Device = strings.ToLower(value)
		case "class":
			info.Class = strings.ToLower(value)
		case "product":
			info.Product = value
		case "memoryMiB":
			info.MemoryMiB = parseMemoryMiB(value)
		}

		devices[index] = info
	}

	result := make([]deviceSnapshot, 0, len(devices))
	for _, device := range devices {
		if device.Vendor == "" || device.Device == "" || device.Class == "" {
			continue
		}
		if device.Vendor != vendorNvidia {
			continue
		}
		result = append(result, device)
	}

	sortDeviceSnapshots(result)
	return result
}

func sortDeviceSnapshots(devices []deviceSnapshot) {
	sort.Slice(devices, func(i, j int) bool {
		left := devices[i].Index
		right := devices[j].Index
		if len(left) == len(right) {
			return left < right
		}
		return left < right
	})
}

func canonicalIndex(index string) string {
	index = strings.TrimSpace(index)
	if index == "" {
		return "0"
	}
	if i, err := strconv.Atoi(index); err == nil {
		return strconv.Itoa(i)
	}
	return index
}

