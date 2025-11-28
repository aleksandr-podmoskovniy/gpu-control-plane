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

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGPUNodeInventoryStatusDeepCopy(t *testing.T) {
	ts := metav1.NewTime(time.Unix(1710000000, 0))

	original := &GPUNodeInventoryStatus{
		Driver: GPUNodeDriver{
			Version:      "535.154.05",
			CUDAVersion:  "12.4",
			ToolkitReady: true,
		},
		Devices: []GPUNodeDevice{{
			InventoryID: "node1-0000:01:00.0",
			UUID:        "GPU-123",
			Product:     "A100",
			Family:      "Ampere",
			PCI: PCIAddress{
				Vendor:  "10de",
				Device:  "20f1",
				Class:   "0302",
				Address: "0000:01:00.0",
			},
			NUMANode:   int32Ptr(0),
			MemoryMiB:  40536,
			MIG:        GPUMIGConfig{Capable: true, Strategy: GPUMIGStrategySingle, ProfilesSupported: []string{"1g.10gb"}},
			ComputeCap: &GPUComputeCapability{Major: 8, Minor: 0},
			State:      GPUDeviceStateReady,
			LastError:  "recent error",
			LastErrorReason: "XID",
			LastUpdatedTime: &ts,
		}},
		Conditions: []metav1.Condition{{
			Type:               "ReadyForPooling",
			Status:             metav1.ConditionTrue,
			Reason:             "OK",
			Message:            "node ready",
			LastTransitionTime: ts,
		}},
	}

	cloned := original.DeepCopy()
	if cloned == original {
		t.Fatal("expected deep copy to allocate a new instance")
	}
	if cloned.Devices[0].NUMANode == original.Devices[0].NUMANode {
		t.Fatal("NUMA pointer should be copied")
	}
	if cloned.Devices[0].ComputeCap == original.Devices[0].ComputeCap {
		t.Fatal("compute capability pointer should be copied")
	}
	if cloned.Devices[0].LastUpdatedTime == original.Devices[0].LastUpdatedTime {
		t.Fatal("timestamp pointers should be copied")
	}

	cloned.Devices[0].PCI.Vendor = "mutated"
	cloned.Devices[0].MIG.ProfilesSupported[0] = "mutated"
	cloned.Conditions[0].Reason = "Changed"
	if original.Devices[0].PCI.Vendor != "10de" {
		t.Fatal("mutating clone should not affect original")
	}
	if original.Devices[0].MIG.ProfilesSupported[0] != "1g.10gb" {
		t.Fatal("profiles slice should be deep-copied")
	}
	if original.Conditions[0].Reason != "OK" {
		t.Fatal("conditions slice should be deep-copied")
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
