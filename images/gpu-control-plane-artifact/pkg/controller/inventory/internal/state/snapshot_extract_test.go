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

import "testing"

func TestExtractDeviceSnapshotsFiltersNonNvidia(t *testing.T) {
	labels := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "abcd",
		"gpu.deckhouse.io/device.00.device": "1234",
		"gpu.deckhouse.io/device.00.class":  "0302",
	}
	if devices := extractDeviceSnapshots(labels); len(devices) != 0 {
		t.Fatalf("expected non-NVIDIA devices to be filtered, got %+v", devices)
	}

	labels["gpu.deckhouse.io/device.00.vendor"] = "10de"
	labels["gpu.deckhouse.io/device.00.memoryMiB"] = "16384"
	devices := extractDeviceSnapshots(labels)
	if len(devices) != 1 || devices[0].MemoryMiB != 16384 {
		t.Fatalf("expected one NVIDIA device, got %+v", devices)
	}
}

func TestExtractDeviceSnapshotsSkipsMalformedEntries(t *testing.T) {
	labels := map[string]string{
		"gpu.deckhouse.io/device.00":               "broken",
		"gpu.deckhouse.io/device.01.vendor":        "10de",
		"gpu.deckhouse.io/device.01.device":        "2230",
		"gpu.deckhouse.io/device.01.class":         "0302",
		"gpu.deckhouse.io/device.02.vendor":        "10de",
		"gpu.deckhouse.io/device.02.device":        "1db5",
		"gpu.deckhouse.io/device.02.class":         "",
		"gpu.deckhouse.io/device.02.memoryMiB":     "11000",
		"gpu.deckhouse.io/device.03.vendor":        "10de",
		"gpu.deckhouse.io/device.03.device":        "1db5",
		"gpu.deckhouse.io/device.03.class":         "0302",
		"gpu.deckhouse.io/device.03.product":       "GPU Product",
		"gpu.deckhouse.io/device.03.memoryMiB":     "12 GiB",
		"gpu.deckhouse.io/device.03.compute.major": "8",
		"gpu.deckhouse.io/device.03.compute.minor": "9",
	}

	devices := extractDeviceSnapshots(labels)
	if len(devices) != 2 {
		t.Fatalf("expected two valid devices, got %+v", devices)
	}
	if devices[0].Index != "1" || devices[1].Index != "3" {
		t.Fatalf("unexpected indices: %+v", devices)
	}
	if devices[1].Product != "GPU Product" || devices[1].MemoryMiB != 12288 {
		t.Fatalf("expected enriched product and memory, got %+v", devices[1])
	}
}
