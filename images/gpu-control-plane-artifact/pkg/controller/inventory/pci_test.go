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

package inventory

import "testing"

func TestCanonicalizePCIAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "trim spaces", in: " 0000:01:00.0 ", want: "0000:01:00.0"},
		{name: "nvml 8 digit domain", in: "00000000:01:00.0", want: "0000:01:00.0"},
		{name: "nvml upper hex", in: "00000000:0A:0B.0", want: "0000:0a:0b.0"},
		{name: "already canonical", in: "0000:65:00.0", want: "0000:65:00.0"},
		{name: "non zero domain", in: "00000001:01:00.0", want: "0001:01:00.0"},
		{name: "missing function delimiter", in: "0000:01:00", want: "0000:01:00"},
		{name: "invalid hex", in: "zzzz:01:00.0", want: "zzzz:01:00.0"},
		{name: "invalid function", in: "0000:01:00.8", want: "0000:01:00.8"},
		{name: "invalid format preserved", in: "not-an-addr", want: "not-an-addr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalizePCIAddress(tt.in)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
