/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package state

import "testing"

func TestVendorLabelFromID(t *testing.T) {
	dev := Device{VendorID: "10de"}
	if got := VendorLabel(dev); got != "nvidia" {
		t.Fatalf("expected nvidia, got %q", got)
	}
}

func TestVendorLabelFromName(t *testing.T) {
	dev := Device{VendorName: "NVIDIA Corporation"}
	if got := VendorLabel(dev); got != "nvidia" {
		t.Fatalf("expected nvidia, got %q", got)
	}
}

func TestDeviceLabelFromBrackets(t *testing.T) {
	if got := DeviceLabel("GA100GL [A30 PCIe]"); got != "a30-pcie" {
		t.Fatalf("expected a30-pcie, got %q", got)
	}
}

func TestDeviceLabelPlain(t *testing.T) {
	if got := DeviceLabel("Tesla V100-PCIE-32GB"); got != "tesla-v100-pcie-32gb" {
		t.Fatalf("expected tesla-v100-pcie-32gb, got %q", got)
	}
}

func TestDeviceLabelEmpty(t *testing.T) {
	if got := DeviceLabel(""); got != "" {
		t.Fatalf("expected empty label, got %q", got)
	}
}

func TestLabelsForDeviceSkipEmpty(t *testing.T) {
	labels := LabelsForDevice("node-1", Device{})
	if labels[LabelNode] != "node-1" {
		t.Fatalf("expected node label, got %q", labels[LabelNode])
	}
	if _, ok := labels[LabelVendor]; ok {
		t.Fatalf("unexpected vendor label")
	}
	if _, ok := labels[LabelDevice]; ok {
		t.Fatalf("unexpected device label")
	}
}
