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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestBuildNodeSnapshotFillsHardwareDefaults(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.0.vendor": "10de",
				"gpu.deckhouse.io/device.0.device": "1db4",
				"gpu.deckhouse.io/device.0.class":  "0300",
				"nvidia.com/gpu.product":           "Test GPU",
				"nvidia.com/gpu.numa.node":         "1",
				"nvidia.com/gpu.power.limit":       "250",
				"nvidia.com/gpu.sm.count":          "64",
				"nvidia.com/gpu.memory.bandwidth":  "1555",
				"nvidia.com/gpu.pcie.gen":          "4",
				"nvidia.com/gpu.pcie.link.width":   "16",
				"nvidia.com/gpu.board":             "board-id",
				"nvidia.com/gpu.family":            "ampere",
				"nvidia.com/gpu.serial":            "serial-1",
				"nvidia.com/gpu.pstate":            "P0",
				"nvidia.com/gpu.display_mode":      "Enabled",
				"nvidia.com/gpu.memory":            "40960 MiB",
				"nvidia.com/gpu.compute.major":     "8",
				"nvidia.com/gpu.compute.minor":     "0",
			},
		},
	}

	snapshot := buildNodeSnapshot(node, nil, ManagedNodesPolicy{EnabledByDefault: true})
	if len(snapshot.Devices) != 1 {
		t.Fatalf("expected one device, got %d", len(snapshot.Devices))
	}
	dev := snapshot.Devices[0]
	assertIntPtr(t, dev.NUMANode, 1)
	assertIntPtr(t, dev.PowerLimitMW, 250)
	assertIntPtr(t, dev.SMCount, 64)
	assertIntPtr(t, dev.MemBandwidth, 1555)
	assertIntPtr(t, dev.PCIEGen, 4)
	assertIntPtr(t, dev.PCIELinkWid, 16)
	if dev.Board != "board-id" || dev.Family != "ampere" || dev.Serial != "serial-1" || dev.PState != "P0" || dev.DisplayMode != "Enabled" {
		t.Fatalf("unexpected board/family/serial/pstate/display: %+v", dev)
	}
}

func TestEnrichDevicesFromFeatureOverridesHardware(t *testing.T) {
	devices := []deviceSnapshot{{
		Index: "0", Vendor: "10de", Device: "1db4", Class: "0300",
		NUMANode: ptrInt32(0),
	}}

	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{
								Attributes: map[string]string{
									"index":            "0",
									"vendor":           "10de",
									"device":           "1db4",
									"class":            "0300",
									"pci.address":      "0000:01:00.0",
									"numa.node":        "2",
									"power.limit":      "200",
									"sm.count":         "80",
									"memory.bandwidth": "1600",
									"pcie.gen":         "5",
									"pcie.link.width":  "8",
									"board":            "board-2",
									"family":           "hopper",
									"serial":           "serial-2",
									"pstate":           "P2",
									"display_mode":     "Disabled",
								},
							},
						},
					},
				},
			},
		},
	}

	res := enrichDevicesFromFeature(devices, feature)
	if len(res) != 1 {
		t.Fatalf("expected one device, got %d", len(res))
	}
	dev := res[0]
	assertIntPtr(t, dev.NUMANode, 2)
	assertIntPtr(t, dev.PowerLimitMW, 200)
	assertIntPtr(t, dev.SMCount, 80)
	assertIntPtr(t, dev.MemBandwidth, 1600)
	assertIntPtr(t, dev.PCIEGen, 5)
	assertIntPtr(t, dev.PCIELinkWid, 8)
	if dev.PCIAddress != "0000:01:00.0" {
		t.Fatalf("expected pci address propagated, got %q", dev.PCIAddress)
	}
	if dev.Board != "board-2" || dev.Family != "hopper" || dev.Serial != "serial-2" || dev.PState != "P2" || dev.DisplayMode != "Disabled" {
		t.Fatalf("unexpected feature board/family/serial/pstate/display: %+v", dev)
	}
}

func TestParseOptionalInt32(t *testing.T) {
	if val := parseOptionalInt32(""); val != nil {
		t.Fatalf("expected nil for empty value, got %v", val)
	}
	val := parseOptionalInt32("42")
	assertIntPtr(t, val, 42)
}

func assertIntPtr(t *testing.T, ptr *int32, expected int32) {
	t.Helper()
	if ptr == nil || *ptr != expected {
		t.Fatalf("expected %d, got %v", expected, ptr)
	}
}

func ptrInt32(v int32) *int32 { return &v }
