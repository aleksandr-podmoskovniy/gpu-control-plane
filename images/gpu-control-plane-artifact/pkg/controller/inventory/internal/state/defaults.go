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

import "strings"

func parseHardwareDefaults(labels map[string]string) deviceSnapshot {
	snapshot := deviceSnapshot{
		Product:      firstNonEmpty(labels[gfdProductLabel]),
		MemoryMiB:    parseMemoryMiB(labels[gfdMemoryLabel]),
		ComputeMajor: parseInt32(labels[gfdComputeMajorLabel]),
		ComputeMinor: parseInt32(labels[gfdComputeMinorLabel]),
		NUMANode:     parseOptionalInt32(labels["nvidia.com/gpu.numa.node"]),
		PowerLimitMW: parseOptionalInt32(labels["nvidia.com/gpu.power.limit"]),
		SMCount:      parseOptionalInt32(labels["nvidia.com/gpu.sm.count"]),
		MemBandwidth: parseOptionalInt32(labels["nvidia.com/gpu.memory.bandwidth"]),
		PCIEGen:      parseOptionalInt32(labels["nvidia.com/gpu.pcie.gen"]),
		PCIELinkWid:  parseOptionalInt32(labels["nvidia.com/gpu.pcie.link.width"]),
		Board:        strings.TrimSpace(labels["nvidia.com/gpu.board"]),
		Family:       strings.TrimSpace(labels["nvidia.com/gpu.family"]),
		Serial:       strings.TrimSpace(labels["nvidia.com/gpu.serial"]),
		PState:       strings.TrimSpace(labels["nvidia.com/gpu.pstate"]),
		DisplayMode:  strings.TrimSpace(labels["nvidia.com/gpu.display_mode"]),
		MIG:          parseMIGConfig(labels),
	}

	if !snapshot.MIG.Capable && len(snapshot.MIG.Types) > 0 {
		snapshot.MIG.Capable = true
	}
	if !snapshot.MIG.Capable && len(snapshot.MIG.ProfilesSupported) > 0 {
		snapshot.MIG.Capable = true
	}

	return snapshot
}

func firstExisting(labels map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := labels[key]; ok {
			return value, true
		}
	}
	return "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

