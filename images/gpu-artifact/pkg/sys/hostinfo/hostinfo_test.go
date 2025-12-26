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

package hostinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	content := "ID=ubuntu\nVERSION_ID=\"22.04\"\nNAME=\"Ubuntu\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write os-release: %v", err)
	}

	release, err := parseOSRelease(path)
	if err != nil {
		t.Fatalf("parse os-release: %v", err)
	}

	if release["ID"] != "ubuntu" {
		t.Fatalf("expected ID ubuntu, got %q", release["ID"])
	}
	if release["VERSION_ID"] != "22.04" {
		t.Fatalf("expected VERSION_ID 22.04, got %q", release["VERSION_ID"])
	}
	if release["NAME"] != "Ubuntu" {
		t.Fatalf("expected NAME Ubuntu, got %q", release["NAME"])
	}
}

func TestIsVirtualizedDMI(t *testing.T) {
	if !isVirtualizedDMI("QEMU", "Standard PC") {
		t.Fatalf("expected QEMU to be detected as virtualized")
	}
	if isVirtualizedDMI("Dell", "PowerEdge R740") {
		t.Fatalf("expected Dell PowerEdge to be treated as bare metal")
	}
}
