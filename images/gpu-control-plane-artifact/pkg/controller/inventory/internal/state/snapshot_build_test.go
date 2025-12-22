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
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

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
