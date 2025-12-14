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

package inventory

import (
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestApplyDetectionNoMatch(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	snapshot := deviceSnapshot{Index: "1", UUID: "missing"}
	applyDetection(device, snapshot, nodeDetection{})
	if device.Status.Hardware.UUID != "" {
		t.Fatalf("hardware should remain untouched when detection missing")
	}
}

func TestApplyDetectionWithTemperatureAndMIG(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	snapshot := deviceSnapshot{Index: "0", UUID: "UUID-1"}
	detections := nodeDetection{
		byUUID: map[string]detectGPUEntry{
			"UUID-1": {
				UUID:                        "UUID-1",
				TemperatureC:                70,
				PowerUsage:                  1000,
				PowerManagementDefaultLimit: 2000,
				MemoryInfo:                  detectGPUMemory{Total: 2, Free: 1, Used: 1},
				Utilization:                 detectGPUUtilization{GPU: 10, Memory: 20},
				MIG:                         detectGPUMIG{Capable: true, ProfilesSupported: []string{"1g.10gb"}},
			},
		},
	}

	applyDetection(device, snapshot, detections)

	if device.Status.Hardware.MIG.Capable == false || len(device.Status.Hardware.MIG.ProfilesSupported) != 1 {
		t.Fatalf("expected MIG capabilities propagated: %+v", device.Status.Hardware.MIG)
	}
}

func TestApplyDetectionHardwareNilFields(t *testing.T) {
	device := &v1alpha1.GPUDevice{}
	entry := detectGPUEntry{}
	applyDetectionHardware(device, entry)
	if device.Status.Hardware.MIG.Capable {
		t.Fatalf("expected MIG capable to remain default false")
	}
}
