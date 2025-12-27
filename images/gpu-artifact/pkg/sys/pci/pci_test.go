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

package pci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/testutil"
)

func TestSysfsReaderList(t *testing.T) {
	root := t.TempDir()
	devicesDir := filepath.Join(root, "bus/pci/devices")
	if err := os.MkdirAll(devicesDir, 0o755); err != nil {
		t.Fatalf("mkdir devices: %v", err)
	}

	gpuDir := filepath.Join(devicesDir, "0000:02:00.0")
	if err := os.MkdirAll(gpuDir, 0o755); err != nil {
		t.Fatalf("mkdir gpu: %v", err)
	}
	testutil.WriteFile(t, filepath.Join(gpuDir, "class"), "0x030200")
	testutil.WriteFile(t, filepath.Join(gpuDir, "vendor"), "0x10de")
	testutil.WriteFile(t, filepath.Join(gpuDir, "device"), "0x20b7")
	if err := os.Symlink("/sys/bus/pci/drivers/nvidia", filepath.Join(gpuDir, "driver")); err != nil {
		t.Fatalf("symlink driver: %v", err)
	}

	reader := NewSysfsReader(root)
	devices, err := reader.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.Address != "0000:02:00.0" {
		t.Fatalf("unexpected address %q", dev.Address)
	}
	if dev.ClassCode != "0302" {
		t.Fatalf("unexpected class code %q", dev.ClassCode)
	}
	if dev.VendorID != "10de" {
		t.Fatalf("unexpected vendor id %q", dev.VendorID)
	}
	if dev.DeviceID != "20b7" {
		t.Fatalf("unexpected device id %q", dev.DeviceID)
	}
	if dev.DriverName != "nvidia" {
		t.Fatalf("unexpected driver name %q", dev.DriverName)
	}
}
