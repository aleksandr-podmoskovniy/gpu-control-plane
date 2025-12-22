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

package service

import (
	"reflect"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func TestApplyDetectionPopulatesHardware(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	snapshot := invstate.DeviceSnapshot{Index: "0", UUID: "GPU-AAA"}
	detections := NodeDetection{
		byUUID: map[string]detectGPUEntry{
			"GPU-AAA": {
				Index:   0,
				UUID:    "GPU-AAA",
				Product: "A100",
				MemoryInfo: detectGPUMemory{
					Total: 80 * 1024 * 1024 * 1024,
					Free:  60 * 1024 * 1024 * 1024,
					Used:  20 * 1024 * 1024 * 1024,
				},
				PowerUsage:                  120000,
				PowerManagementDefaultLimit: 150000,
				Utilization: detectGPUUtilization{
					GPU:    75,
					Memory: 40,
				},
				MemoryMiB:          80 * 1024,
				ComputeMajor:       8,
				ComputeMinor:       0,
				NUMANode:           detectionPtrInt32(1),
				SMCount:            detectionPtrInt32(108),
				MemoryBandwidthMiB: detectionPtrInt32(1555),
				PCI: detectGPUPCI{
					Address: "0000:17:00.0",
					Vendor:  "10de",
					Device:  "2203",
					Class:   "0302",
				},
				Board:       "board-1",
				Family:      "ampere",
				Serial:      "serial-123",
				DisplayMode: "Enabled",
				MIG:         detectGPUMIG{Capable: true, ProfilesSupported: []string{"mig-1g.10gb"}},
				PowerState:  0,
			},
		},
		byIndex: map[string]detectGPUEntry{},
	}

	ApplyDetection(device, snapshot, detections)

	if device.Status.Hardware.Product != "A100" || device.Status.Hardware.PCI.Vendor != "10de" || device.Status.Hardware.PCI.Device != "2203" {
		t.Fatalf("unexpected hardware update: %+v", device.Status.Hardware)
	}
	if !device.Status.Hardware.MIG.Capable || len(device.Status.Hardware.MIG.ProfilesSupported) != 1 || device.Status.Hardware.MIG.ProfilesSupported[0] != "1g.10gb" {
		t.Fatalf("expected MIG profiles propagated, got %+v", device.Status.Hardware.MIG)
	}
}

func TestApplyDetectionMissingEntriesDoesNothing(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	snapshot := invstate.DeviceSnapshot{Index: "10", UUID: "GPU-ZZZ"}
	before := device.Status.Hardware
	ApplyDetection(device, snapshot, NodeDetection{})
	if !reflect.DeepEqual(before, device.Status.Hardware) {
		t.Fatalf("hardware should stay untouched without matching detection: before=%+v after=%+v", before, device.Status.Hardware)
	}
}

func TestNodeDetectionFallbacks(t *testing.T) {
	detections := NodeDetection{
		byUUID: map[string]detectGPUEntry{
			"GPU-A": {UUID: "GPU-A", Index: 1},
		},
		byIndex: map[string]detectGPUEntry{
			"5": {Index: 5, UUID: "GPU-B"},
		},
	}

	if entry, ok := detections.find(invstate.DeviceSnapshot{Index: "0", UUID: "GPU-A"}); !ok || entry.UUID != "GPU-A" {
		t.Fatalf("expected lookup by UUID, got %+v ok=%v", entry, ok)
	}
	if entry, ok := detections.find(invstate.DeviceSnapshot{Index: "5"}); !ok || entry.Index != 5 {
		t.Fatalf("expected lookup by index, got %+v ok=%v", entry, ok)
	}
	if _, ok := detections.find(invstate.DeviceSnapshot{Index: "99"}); ok {
		t.Fatalf("unexpected entry for missing snapshot")
	}
}

func detectionPtrInt32(v int32) *int32 { return &v }
