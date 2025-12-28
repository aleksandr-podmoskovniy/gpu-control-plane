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

package pciids

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolverLookups(t *testing.T) {
	path := filepath.Join("testdata", "pci.ids")
	res, err := Load(path)
	if err != nil {
		t.Fatalf("load pci.ids: %v", err)
	}

	if got := res.VendorName("10de"); got != "NVIDIA Corporation" {
		t.Fatalf("unexpected vendor name %q", got)
	}
	if got := res.DeviceName("10de", "20b7"); got != "GA100GL [A30 PCIe]" {
		t.Fatalf("unexpected device name %q", got)
	}
	if got := res.ClassName("0302"); got != "3D controller" {
		t.Fatalf("unexpected class name %q", got)
	}
	if got := res.ClassName("0300"); got != "VGA compatible controller" {
		t.Fatalf("unexpected class name %q", got)
	}
}

func TestResolverLookupsWithSpaceIndent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pci.ids")
	content := []byte("10de  NVIDIA Corporation\n  2203  GA102 [GeForce RTX 3090 Ti]\nC 03  Display controller\n  02  3D controller\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write pci.ids: %v", err)
	}

	res, err := Load(path)
	if err != nil {
		t.Fatalf("load pci.ids: %v", err)
	}
	if got := res.DeviceName("10de", "2203"); got != "GA102 [GeForce RTX 3090 Ti]" {
		t.Fatalf("unexpected device name %q", got)
	}
	if got := res.ClassName("0302"); got != "3D controller" {
		t.Fatalf("unexpected class name %q", got)
	}
}
