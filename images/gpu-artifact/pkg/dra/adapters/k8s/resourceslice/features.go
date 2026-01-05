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

// FeatureSet controls which ResourceSlice features are rendered.
type FeatureSet struct {
	PartitionableDevices bool
	ConsumableCapacity   bool
}

// DefaultFeatures enables partitionable devices and keeps consumable capacity disabled.
func DefaultFeatures() FeatureSet {
	return FeatureSet{
		PartitionableDevices: true,
		ConsumableCapacity:   false,
	}
}

// Enable returns a copy with the named features enabled.
func (f FeatureSet) Enable(features []string) (FeatureSet, bool) {
	updated := f
	changed := false
	for _, feature := range features {
		switch feature {
		case "DRAPartitionableDevices":
			if !updated.PartitionableDevices {
				updated.PartitionableDevices = true
				changed = true
			}
		case "DRAConsumableCapacity":
			if !updated.ConsumableCapacity {
				updated.ConsumableCapacity = true
				changed = true
			}
		}
	}
	return updated, changed
}

// Disable returns a copy with the named features disabled.
func (f FeatureSet) Disable(features []string) (FeatureSet, bool) {
	updated := f
	changed := false
	for _, feature := range features {
		switch feature {
		case "DRAPartitionableDevices":
			if updated.PartitionableDevices {
				updated.PartitionableDevices = false
				changed = true
			}
		case "DRAConsumableCapacity":
			if updated.ConsumableCapacity {
				updated.ConsumableCapacity = false
				changed = true
			}
		}
	}
	return updated, changed
}
