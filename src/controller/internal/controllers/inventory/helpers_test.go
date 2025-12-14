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

import (
	"testing"
)

func TestStringSlicesEqual(t *testing.T) {
	if !stringSlicesEqual(nil, nil) {
		t.Fatal("expected two nil slices to be equal")
	}
	if stringSlicesEqual([]string{"a"}, []string{"b"}) {
		t.Fatal("expected different slices to be unequal")
	}
	if stringSlicesEqual([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("expected slices with different lengths to be unequal")
	}
	if !stringSlicesEqual([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("expected same content slices to be equal")
	}
}
