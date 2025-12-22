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

package common

import "testing"

func TestAppNameFormatting(t *testing.T) {
	got := AppName(ComponentValidator)
	if got != "nvidia-operator-validator" {
		t.Fatalf("unexpected app name: %s", got)
	}
}

func TestComponentAppNamesReturnsCopy(t *testing.T) {
	names := ComponentAppNames()
	if len(names) != len(managedComponents) {
		t.Fatalf("expected %d names, got %d", len(managedComponents), len(names))
	}
	names[0] = "modified"
	if ComponentAppNames()[0] == "modified" {
		t.Fatal("expected copy to be returned")
	}
}
