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

package validators

import "testing"

func TestIsHex4(t *testing.T) {
	if isHex4("123") {
		t.Fatalf("expected len!=4 to be invalid")
	}
	if isHex4("zzzz") {
		t.Fatalf("expected non-hex chars to be invalid")
	}
	if !isHex4("10de") {
		t.Fatalf("expected valid hex")
	}
}

func TestIsValidMIGProfile(t *testing.T) {
	if isValidMIGProfile("bad") {
		t.Fatalf("expected invalid mig profile")
	}
	if !isValidMIGProfile("1g.10gb") {
		t.Fatalf("expected valid mig profile")
	}
}

func TestDedupStringsHelper(t *testing.T) {
	out := dedupStrings([]string{"", " ", "a", "a", " b "})
	if len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("unexpected dedup result: %v", out)
	}
	if res := dedupStrings(nil); len(res) != 0 {
		t.Fatalf("expected empty slice for nil input, got %v", res)
	}
}
