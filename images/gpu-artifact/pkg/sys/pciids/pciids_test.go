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
