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

func TestWarningCollector(t *testing.T) {
	var w warningCollector
	w.addf("hello %s %d", "gpu", 1)
	if len(w) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(w))
	}
	if w[0] != "hello gpu 1" {
		t.Fatalf("unexpected warning text: %s", w[0])
	}
}
