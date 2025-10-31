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

package contracts

import "testing"

type stubNamed struct{ id string }

func (s stubNamed) Name() string { return s.id }

func TestRegistryRegisterAndList(t *testing.T) {
	r := NewRegistry[stubNamed]()
	first := stubNamed{id: "first"}
	second := stubNamed{id: "second"}

	r.Register(first)
	r.Register(second)

	items := r.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Register same name should replace
	r.Register(stubNamed{id: "first"})
	items = r.List()
	if len(items) != 2 {
		t.Fatalf("expected replacement to keep 2 items, got %d", len(items))
	}
}
