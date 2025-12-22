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
	"reflect"
	"strings"
	"testing"
)

func TestSortDeviceSnapshotsOrdersIndices(t *testing.T) {
	devices := []deviceSnapshot{{Index: "10"}, {Index: "2"}, {Index: "001"}}
	sortDeviceSnapshots(devices)
	got := []string{devices[0].Index, devices[1].Index, devices[2].Index}
	want := []string{"001", "10", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestDeduplicateStrings(t *testing.T) {
	input := []string{"fp32", "fp16", "fp32", "tf32"}
	deduped := deduplicateStrings(input)
	if !reflect.DeepEqual(deduped, []string{"fp32", "fp16", "tf32"}) {
		t.Fatalf("unexpected deduplicate result: %v", deduped)
	}
}

func TestCanonicalIndexVariants(t *testing.T) {
	cases := map[string]string{
		"":    "0",
		"01":  "1",
		"A12": "A12",
	}
	for in, want := range cases {
		if got := canonicalIndex(in); got != want {
			t.Fatalf("canonicalIndex(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestTruncateNameLimitsLength(t *testing.T) {
	long := strings.Repeat("a", 80)
	if len(truncateName(long)) != 63 {
		t.Fatalf("expected name to be truncated to 63 characters")
	}
}

func TestBuildDeviceAndInventoryNames(t *testing.T) {
	info := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5"}
	name := buildDeviceName("Node_A", info)
	if !strings.HasPrefix(name, "node-a-0-") {
		t.Fatalf("unexpected device name %s", name)
	}
	inv := buildInventoryID("Node_A", info)
	if inv != name {
		t.Fatalf("expected inventory ID to match device name, got %s", inv)
	}
	fallback := buildDeviceName("###", deviceSnapshot{})
	if fallback != "gpu-gpu" {
		t.Fatalf("expected fallback device name, got %s", fallback)
	}
}
