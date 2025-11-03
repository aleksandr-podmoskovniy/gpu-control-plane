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

package config

import (
	"testing"

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestModuleConfigStoreCloneOnCurrent(t *testing.T) {
	state := moduleconfig.DefaultState()
	store := NewModuleConfigStore(state)

	current := store.Current()
	current.Settings.ManagedNodes.LabelKey = "modified"

	next := store.Current()
	if next.Settings.ManagedNodes.LabelKey == "modified" {
		t.Fatalf("store returned state must not be affected by external mutations")
	}
}

func TestModuleConfigStoreCloneOnUpdate(t *testing.T) {
	store := NewModuleConfigStore(moduleconfig.DefaultState())
	state := moduleconfig.DefaultState()
	state.Settings.ManagedNodes.LabelKey = "custom"

	store.Update(state)

	state.Settings.ManagedNodes.LabelKey = "changed-after-update"

	current := store.Current()
	if current.Settings.ManagedNodes.LabelKey != "custom" {
		t.Fatalf("Update must clone state, got %s", current.Settings.ManagedNodes.LabelKey)
	}
}
