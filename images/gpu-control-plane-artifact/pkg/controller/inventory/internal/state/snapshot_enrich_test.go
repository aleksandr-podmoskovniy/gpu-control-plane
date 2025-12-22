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
	"testing"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

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
