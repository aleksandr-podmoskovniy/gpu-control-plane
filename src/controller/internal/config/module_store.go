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
	"sync"

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

// ModuleConfigStore keeps the current module State for controllers.
type ModuleConfigStore struct {
	mu    sync.RWMutex
	state moduleconfig.State
}

// NewModuleConfigStore initialises store with provided state.
func NewModuleConfigStore(state moduleconfig.State) *ModuleConfigStore {
	return &ModuleConfigStore{state: state.Clone()}
}

// Current returns a copy of the current state.
func (s *ModuleConfigStore) Current() moduleconfig.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Clone()
}

// Update replaces stored state.
func (s *ModuleConfigStore) Update(state moduleconfig.State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state.Clone()
}
