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

import (
	"strings"
	"testing"
)

func TestPhysicalGPUName(t *testing.T) {
	dev := Device{
		Index:    "0",
		VendorID: "10de",
		DeviceID: "20b7",
	}
	if got := PhysicalGPUName("k8s-w1-gpu", dev); got != "k8s-w1-gpu-0-10de-20b7" {
		t.Fatalf("expected k8s-w1-gpu-0-10de-20b7, got %q", got)
	}
}

func TestPhysicalGPUNameSanitize(t *testing.T) {
	dev := Device{
		Index:    "1",
		VendorID: "10DE",
		DeviceID: "20B7",
	}
	if got := PhysicalGPUName("K8S_W1@GPU", dev); got != "k8s-w1-gpu-1-10de-20b7" {
		t.Fatalf("expected k8s-w1-gpu-1-10de-20b7, got %q", got)
	}
}

func TestPhysicalGPUNameTruncate(t *testing.T) {
	longNode := strings.Repeat("a", 300)
	name := PhysicalGPUName(longNode, Device{Index: "0", VendorID: "10de", DeviceID: "20b7"})
	if len(name) > maxNameLength {
		t.Fatalf("expected truncated name, got length %d", len(name))
	}
}
