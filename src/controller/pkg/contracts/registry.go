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

import "sync"

// Named describes an extension that can be referenced by name.
type Named interface {
	Name() string
}

// Registry stores implementations keyed by their name and allows safe concurrent access.
type Registry[T Named] struct {
	mu    sync.RWMutex
	items map[string]T
}

// NewRegistry creates an empty registry.
func NewRegistry[T Named]() *Registry[T] {
	return &Registry[T]{
		items: make(map[string]T),
	}
}

// Register adds or replaces an implementation.
func (r *Registry[T]) Register(handler T) {
	r.mu.Lock()
	r.items[handler.Name()] = handler
	r.mu.Unlock()
}

// List returns a snapshot of registered implementations.
func (r *Registry[T]) List() []T {
	r.mu.RLock()
	result := make([]T, 0, len(r.items))
	for _, v := range r.items {
		result = append(result, v)
	}
	r.mu.RUnlock()
	return result
}
