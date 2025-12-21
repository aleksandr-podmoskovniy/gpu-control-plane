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
	"reflect"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func defaultManagedPolicy() ManagedNodesPolicy {
	return ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/enabled", EnabledByDefault: true}
}

func TestBuildNodeSnapshotMergesNodeAndFeatureLabels(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "20b0",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.memory":             "40960 MiB",
			},
		},
	}

	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"nvidia.com/gpu.product":              "NVIDIA RTX A6000",
				"nvidia.com/gpu.compute.major":        "8",
				"nvidia.com/gpu.compute.minor":        "6",
				"nvidia.com/gpu.driver":               "535.104.05",
				"nvidia.com/cuda.driver.major":        "12",
				"nvidia.com/cuda.driver.minor":        "3",
				"nvidia.com/mig-1g.10gb.count":        "1",
				"nvidia.com/mig-1g.10gb.engines.copy": "3",
				"gpu.deckhouse.io/toolkit.ready":      "true",
			},
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":          "0",
								"uuid":           "GPU-RTX-A6000-UUID",
								"precision":      "fp32,fp16,tf32",
								"precision.bf16": "true",
							}},
						},
					},
				},
			},
		},
	}

	snapshot := buildNodeSnapshot(node, feature, defaultManagedPolicy())
	if !snapshot.Managed {
		t.Fatal("expected managed=true")
	}
	if len(snapshot.Devices) != 1 {
		t.Fatalf("expected single device, got %d", len(snapshot.Devices))
	}
	device := snapshot.Devices[0]
	if device.Product != "NVIDIA RTX A6000" {
		t.Fatalf("unexpected product: %s", device.Product)
	}
	if device.MemoryMiB != 40960 {
		t.Fatalf("unexpected memory: %d", device.MemoryMiB)
	}
	if device.ComputeMajor != 8 || device.ComputeMinor != 6 {
		t.Fatalf("unexpected compute capability: %d.%d", device.ComputeMajor, device.ComputeMinor)
	}
	if !device.MIG.Capable {
		t.Fatal("expected MIG capable true")
	}
	if len(device.MIG.Types) != 1 {
		t.Fatalf("expected one MIG type, got %d", len(device.MIG.Types))
	}
	typeInfo := device.MIG.Types[0]
	if typeInfo.Name != "1g.10gb" || typeInfo.Count != 1 {
		t.Fatalf("unexpected MIG type: %+v", typeInfo)
	}
	if !snapshot.FeatureDetected {
		t.Fatal("expected feature detected flag set")
	}
	if snapshot.Driver.Version != "535.104.05" {
		t.Fatalf("unexpected driver version: %s", snapshot.Driver.Version)
	}
	if snapshot.Driver.CUDAVersion != "12.3" {
		t.Fatalf("unexpected cuda version: %s", snapshot.Driver.CUDAVersion)
	}
	if !snapshot.Driver.ToolkitReady {
		t.Fatal("expected toolkit ready true")
	}
	if snapshot.Devices[0].UUID != "GPU-RTX-A6000-UUID" {
		t.Fatalf("unexpected device UUID: %s", snapshot.Devices[0].UUID)
	}
	expectedPrecision := []string{"bf16", "fp16", "fp32", "tf32"}
	if !slices.Equal(snapshot.Devices[0].Precision, expectedPrecision) {
		t.Fatalf("unexpected precision list: %+v", snapshot.Devices[0].Precision)
	}
}

func TestBuildNodeSnapshotManagedDisabled(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-2",
			Labels: map[string]string{
				"gpu.deckhouse.io/enabled":          "false",
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	snapshot := buildNodeSnapshot(node, nil, defaultManagedPolicy())
	if snapshot.Managed {
		t.Fatal("expected managed=false when label set to false")
	}
	if snapshot.FeatureDetected {
		t.Fatal("expected feature detected to be false without NodeFeature")
	}
}

func TestCanonicalIndexNormalization(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-3",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1aef",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"gpu.deckhouse.io/device.01.vendor": "10de",
				"gpu.deckhouse.io/device.01.device": "1aef",
				"gpu.deckhouse.io/device.01.class":  "0302",
			},
		},
	}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{"index": "0"}},
							{Attributes: map[string]string{"index": "1", "product": "NVIDIA Test GPU"}},
						},
					},
				},
			},
		},
	}

	snapshot := buildNodeSnapshot(node, feature, defaultManagedPolicy())
	if len(snapshot.Devices) != 2 {
		t.Fatalf("expected two devices, got %d", len(snapshot.Devices))
	}
	if snapshot.Devices[0].Index != "0" || snapshot.Devices[1].Index != "1" {
		t.Fatalf("unexpected indices: %+v", snapshot.Devices)
	}
	if snapshot.Devices[1].Product != "NVIDIA Test GPU" {
		t.Fatalf("expected product from NodeFeature, got %q", snapshot.Devices[1].Product)
	}
}

func TestCatalogFallbackProvidesProductName(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-catalog",
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db6",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	snapshot := buildNodeSnapshot(node, nil, defaultManagedPolicy())
	if len(snapshot.Devices) != 1 {
		t.Fatalf("expected single device, got %d", len(snapshot.Devices))
	}
	if snapshot.Devices[0].Product != "GV100GL [Tesla V100 PCIe 32GB]" {
		t.Fatalf("expected product from catalog, got %q", snapshot.Devices[0].Product)
	}
}

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

func TestEnrichDevicesFromFeatureCreatesMissingDevices(t *testing.T) {
	devices := []deviceSnapshot{
		{Index: "0", Vendor: "10de", Device: "1db6", Class: "0302"},
	}

	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":        "0",
								"uuid":         "GPU-0",
								"memory.total": "16384 MiB",
							}},
							{Attributes: map[string]string{
								"index":  "1",
								"vendor": "10de",
								"device": "2230",
								"class":  "0300",
								"uuid":   "GPU-1",
							}},
							{Attributes: map[string]string{
								"index": "2",
								"uuid":  "GPU-2",
								// missing vendor/device/class -> should be skipped
							}},
						},
					},
				},
			},
		},
	}

	enriched := enrichDevicesFromFeature(devices, feature)
	if len(enriched) != 2 {
		t.Fatalf("expected two devices after enrichment, got %+v", enriched)
	}
	sortDeviceSnapshots(enriched)

	if enriched[1].Index != "1" || enriched[1].Vendor != "10de" || enriched[1].Device != "2230" || enriched[1].UUID != "GPU-1" {
		t.Fatalf("unexpected device created from feature: %+v", enriched[1])
	}
}

func TestEnrichDevicesFromFeatureSkipsEmptyAttributes(t *testing.T) {
	devices := []deviceSnapshot{{Index: "0", Vendor: "10de", Device: "1db5", Class: "0300"}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: nil},
							{Attributes: map[string]string{"index": "1", "vendor": "", "device": ""}},
						},
					},
				},
			},
		},
	}

	enriched := enrichDevicesFromFeature(devices, feature)
	if len(enriched) != 1 || enriched[0].Index != "0" {
		t.Fatalf("expected devices untouched, got %+v", enriched)
	}
}

func TestSortDeviceSnapshotsOrdersIndices(t *testing.T) {
	devices := []deviceSnapshot{{Index: "10"}, {Index: "2"}, {Index: "001"}}
	sortDeviceSnapshots(devices)
	got := []string{devices[0].Index, devices[1].Index, devices[2].Index}
	want := []string{"001", "10", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestDeduplicateStrings(t *testing.T) {
	input := []string{"fp32", "fp16", "fp32", "tf32"}
	deduped := deduplicateStrings(input)
	if !reflect.DeepEqual(deduped, []string{"fp32", "fp16", "tf32"}) {
		t.Fatalf("unexpected deduplicate result: %v", deduped)
	}
}

func TestCanonicalIndexVariants(t *testing.T) {
	cases := map[string]string{
		"":    "0",
		"01":  "1",
		"A12": "A12",
	}
	for in, want := range cases {
		if got := canonicalIndex(in); got != want {
			t.Fatalf("canonicalIndex(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestTruncateNameLimitsLength(t *testing.T) {
	long := strings.Repeat("a", 80)
	if len(truncateName(long)) != 63 {
		t.Fatalf("expected name to be truncated to 63 characters")
	}
}

func TestBuildDeviceAndInventoryNames(t *testing.T) {
	info := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5"}
	name := buildDeviceName("Node_A", info)
	if !strings.HasPrefix(name, "node-a-0-") {
		t.Fatalf("unexpected device name %s", name)
	}
	inv := buildInventoryID("Node_A", info)
	if inv != name {
		t.Fatalf("expected inventory ID to match device name, got %s", inv)
	}
	fallback := buildDeviceName("###", deviceSnapshot{})
	if fallback != "gpu-gpu" {
		t.Fatalf("expected fallback device name, got %s", fallback)
	}
}

func TestEnrichDevicesFromFeatureIgnoresUnknownIndex(t *testing.T) {
	devices := []deviceSnapshot{{Index: "0"}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{"index": "1", "uuid": "ignored"}},
						},
					},
				},
			},
		},
	}
	result := enrichDevicesFromFeature(devices, feature)
	if len(result) != 1 || result[0].UUID != "" {
		t.Fatalf("expected device without matching index unchanged, got %+v", result)
	}
}

func TestEnrichDevicesFromFeaturePropagatesAttributes(t *testing.T) {
	devices := []deviceSnapshot{{Index: "0"}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":          "0",
								"uuid":           "GPU-123",
								"product":        "Feature Product",
								"precision":      "fp32,tf32",
								"precision.bf16": "true",
							}},
						},
					},
				},
			},
		},
	}
	result := enrichDevicesFromFeature(devices, feature)
	if len(result) != 1 {
		t.Fatalf("expected single device, got %+v", result)
	}
	if result[0].UUID != "GPU-123" || result[0].Product != "Feature Product" {
		t.Fatalf("unexpected enrichment result %+v", result[0])
	}
	if !reflect.DeepEqual(result[0].Precision, []string{"bf16", "fp32", "tf32"}) {
		t.Fatalf("expected precision to be normalised, got %+v", result[0].Precision)
	}
}

func TestEnrichDevicesFromFeatureMissingGPUKey(t *testing.T) {
	devices := []deviceSnapshot{{Index: "0"}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"example.com/other": {},
				},
			},
		},
	}
	result := enrichDevicesFromFeature(devices, feature)
	if len(result) != 1 || !reflect.DeepEqual(result[0], devices[0]) {
		t.Fatalf("expected devices unchanged when GPU instance missing, got %+v", result)
	}
}

func TestEnrichDevicesFromFeatureOverridesMetrics(t *testing.T) {
	devices := []deviceSnapshot{{Index: "5"}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":          "5",
								"memory.total":   "24576 MiB",
								"compute.major":  "9",
								"compute.minor":  "9",
								"product":        "Feature GPU",
								"precision":      "fp64",
								"precision.fp32": "true",
							}},
							{Attributes: nil},
							{Attributes: map[string]string{"index": ""}},
						},
					},
				},
			},
		},
	}
	result := enrichDevicesFromFeature(devices, feature)
	if len(result) != 1 {
		t.Fatalf("expected device to be updated, got %+v", result)
	}
	device := result[0]
	if device.MemoryMiB != 24576 || device.ComputeMajor != 9 || device.ComputeMinor != 9 {
		t.Fatalf("expected metrics override, got %+v", device)
	}
	if device.Product != "Feature GPU" {
		t.Fatalf("expected product override, got %s", device.Product)
	}
	if !reflect.DeepEqual(device.Precision, []string{"fp32", "fp64"}) {
		t.Fatalf("expected precision override, got %+v", device.Precision)
	}
}

func TestEnrichDevicesFromFeatureFillsMissingIds(t *testing.T) {
	devices := []deviceSnapshot{{Index: "1"}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":            "1",
								"vendor":           "10DE",
								"device":           "2203",
								"class":            "0300",
								"pci.address":      "0000:01:00.0",
								"memory.total":     "24576 MiB",
								"compute.major":    "8",
								"compute.minor":    "0",
								"numa.node":        "0",
								"power.limit":      "250",
								"sm.count":         "108",
								"memory.bandwidth": "1500",
								"pcie.gen":         "4",
								"pcie.link.width":  "16",
								"board":            "PG132",
								"family":           "Ampere",
								"serial":           "ABC123",
								"pstate":           "P0",
								"display_mode":     "Disabled",
								"precision":        "fp16,fp32",
							}},
						},
					},
				},
			},
		},
	}

	result := enrichDevicesFromFeature(devices, feature)
	if len(result) != 1 {
		t.Fatalf("expected one device, got %+v", result)
	}
	dev := result[0]
	if dev.Vendor != "10de" || dev.Device != "2203" || dev.Class != "0300" {
		t.Fatalf("expected pci ids set, got %+v", dev)
	}
	if dev.PCIAddress != "0000:01:00.0" || dev.MemoryMiB != 24576 || dev.ComputeMajor != 8 || dev.ComputeMinor != 0 {
		t.Fatalf("expected metrics filled, got %+v", dev)
	}
	if dev.NUMANode == nil || *dev.NUMANode != 0 {
		t.Fatalf("expected NUMA set, got %+v", dev.NUMANode)
	}
	if dev.PowerLimitMW == nil || *dev.PowerLimitMW != 250 || dev.SMCount == nil || *dev.SMCount != 108 || dev.MemBandwidth == nil || *dev.MemBandwidth != 1500 {
		t.Fatalf("expected power/SM/bandwidth set, got %+v", dev)
	}
	if dev.PCIEGen == nil || *dev.PCIEGen != 4 || dev.PCIELinkWid == nil || *dev.PCIELinkWid != 16 {
		t.Fatalf("expected pcie fields set, got gen %v width %v", dev.PCIEGen, dev.PCIELinkWid)
	}
	if dev.Board != "PG132" || dev.Family != "Ampere" || dev.Serial != "ABC123" || dev.PState != "P0" || dev.DisplayMode != "Disabled" {
		t.Fatalf("expected board/family/serial/pstate/display set, got %+v", dev)
	}
	if !reflect.DeepEqual(dev.Precision, []string{"fp16", "fp32"}) {
		t.Fatalf("expected precision set, got %+v", dev.Precision)
	}
}
