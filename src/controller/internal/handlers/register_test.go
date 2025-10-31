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

package handlers

import (
	"testing"

	"github.com/go-logr/logr/testr"
)

func TestRegisterDefaultsInitialisesRegistries(t *testing.T) {
	deps := &Handlers{}

	RegisterDefaults(testr.New(t), deps)

	if deps.Inventory == nil || len(deps.Inventory.List()) == 0 {
		t.Fatal("inventory registry must be initialised with default handlers")
	}
	if deps.Bootstrap == nil || len(deps.Bootstrap.List()) == 0 {
		t.Fatal("bootstrap registry must be initialised with default handlers")
	}
	if deps.Pool == nil || len(deps.Pool.List()) == 0 {
		t.Fatal("pool registry must be initialised with default handlers")
	}
	if deps.Admission == nil || len(deps.Admission.List()) == 0 {
		t.Fatal("admission registry must be initialised with default handlers")
	}
}
