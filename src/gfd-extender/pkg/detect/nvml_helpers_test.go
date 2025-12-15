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

package detect

import "testing"

func TestDecodeNVMLPciDeviceID(t *testing.T) {
	// Example from go-nvml mock: 0x20B010DE => vendor 10de, device 20b0.
	vendor, device := decodeNVMLPciDeviceID(0x20B010DE)
	if vendor != "10de" || device != "20b0" {
		t.Fatalf("unexpected decode result: vendor=%s device=%s", vendor, device)
	}
}

func TestMIGInfoFromGetMigMode(t *testing.T) {
	capable, mode := migInfoFromGetMigMode(false, 0)
	if capable || mode != "" {
		t.Fatalf("unsupported mig must return empty info: capable=%v mode=%q", capable, mode)
	}

	capable, mode = migInfoFromGetMigMode(true, 0)
	if !capable || mode != "disabled" {
		t.Fatalf("expected disabled mig, got capable=%v mode=%q", capable, mode)
	}

	capable, mode = migInfoFromGetMigMode(true, 1)
	if !capable || mode != "enabled" {
		t.Fatalf("expected enabled mig, got capable=%v mode=%q", capable, mode)
	}

	capable, mode = migInfoFromGetMigMode(true, 42)
	if !capable || mode != "42" {
		t.Fatalf("expected numeric mig mode, got capable=%v mode=%q", capable, mode)
	}
}
