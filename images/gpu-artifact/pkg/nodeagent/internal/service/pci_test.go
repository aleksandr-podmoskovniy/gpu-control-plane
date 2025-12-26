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

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSysfsPCIProviderScan(t *testing.T) {
	root := t.TempDir()
	devicesDir := filepath.Join(root, "bus/pci/devices")
	if err := os.MkdirAll(devicesDir, 0o755); err != nil {
		t.Fatalf("mkdir devices: %v", err)
	}

	gpuDir := filepath.Join(devicesDir, "0000:01:00.0")
	if err := os.MkdirAll(gpuDir, 0o755); err != nil {
		t.Fatalf("mkdir gpu: %v", err)
	}
	writeFile(t, filepath.Join(gpuDir, "class"), "0x0300")
	writeFile(t, filepath.Join(gpuDir, "vendor"), "0x10de")
	writeFile(t, filepath.Join(gpuDir, "device"), "0x1eb8")

	netDir := filepath.Join(devicesDir, "0000:02:00.0")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		t.Fatalf("mkdir net: %v", err)
	}
	writeFile(t, filepath.Join(netDir, "class"), "0x0200")
	writeFile(t, filepath.Join(netDir, "vendor"), "0x8086")
	writeFile(t, filepath.Join(netDir, "device"), "0x100e")

	provider := NewSysfsPCIProvider(root, nil)
	devices, err := provider.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.Address != "0000:01:00.0" {
		t.Fatalf("unexpected address %q", dev.Address)
	}
	if dev.ClassCode != "0300" {
		t.Fatalf("unexpected class code %q", dev.ClassCode)
	}
	if dev.VendorID != "10de" {
		t.Fatalf("unexpected vendor id %q", dev.VendorID)
	}
	if dev.DeviceID != "1eb8" {
		t.Fatalf("unexpected device id %q", dev.DeviceID)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
