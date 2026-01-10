/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resourceslice

import k8sresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/resourceslice"

// EnableFeatures enables features if present and reports whether a change occurred.
func (b *Builder) EnableFeatures(features []string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	updated, changed := b.features.Enable(features)
	if changed {
		b.features = updated
	}
	return changed
}

// DisableFeatures disables features if present and reports whether a change occurred.
func (b *Builder) DisableFeatures(features []string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	updated, changed := b.features.Disable(features)
	if changed {
		b.features = updated
	}
	return changed
}

// SetSharedCountersLayout updates how shared counters are published.
func (b *Builder) SetSharedCountersLayout(layout k8sresourceslice.SharedCountersLayout) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	updated, changed := b.features.WithSharedCountersLayout(layout)
	if changed {
		b.features = updated
	}
	return changed
}

// SetBindingConditionsEnabled toggles binding conditions rendering.
func (b *Builder) SetBindingConditionsEnabled(enabled bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	updated, changed := b.features.WithBindingConditions(enabled)
	if changed {
		b.features = updated
	}
	return changed
}
