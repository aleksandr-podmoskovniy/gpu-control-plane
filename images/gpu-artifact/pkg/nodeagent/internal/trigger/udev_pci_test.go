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

package trigger

import "testing"

func TestParseUEvent(t *testing.T) {
	t.Parallel()

	payload := []byte("add@/devices/pci0000:00/0000:02:00.0\x00SUBSYSTEM=pci\x00ACTION=add\x00DEVPATH=/devices/pci0000:00/0000:02:00.0\x00INVALID\x00")
	env := parseUEvent(payload)

	if env["SUBSYSTEM"] != "pci" {
		t.Fatalf("expected SUBSYSTEM=pci, got %q", env["SUBSYSTEM"])
	}
	if env["ACTION"] != "add" {
		t.Fatalf("expected ACTION=add, got %q", env["ACTION"])
	}
	if env["DEVPATH"] == "" {
		t.Fatalf("expected DEVPATH to be set")
	}
	if _, ok := env["INVALID"]; ok {
		t.Fatalf("expected invalid entry to be skipped")
	}
}
