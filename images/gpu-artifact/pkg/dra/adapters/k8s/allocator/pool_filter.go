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

package allocator

import (
	"sort"

	resourcev1 "k8s.io/api/resource/v1"
)

func filterPoolSlices(driverName string, slices []resourcev1.ResourceSlice) []resourcev1.ResourceSlice {
	type poolGen struct {
		expected int64
		slices   []resourcev1.ResourceSlice
	}
	type poolEntry struct {
		generation int64
		data       *poolGen
	}

	pools := map[string]map[int64]*poolGen{}
	for _, slice := range slices {
		if slice.Spec.Driver != driverName {
			continue
		}
		poolName := slice.Spec.Pool.Name
		if poolName == "" {
			continue
		}
		expected := slice.Spec.Pool.ResourceSliceCount
		if expected < 1 {
			continue
		}
		gen := slice.Spec.Pool.Generation
		if pools[poolName] == nil {
			pools[poolName] = map[int64]*poolGen{}
		}
		entry := pools[poolName][gen]
		if entry == nil {
			entry = &poolGen{expected: expected}
			pools[poolName][gen] = entry
		}
		entry.slices = append(entry.slices, slice)
	}

	valid := make([]resourcev1.ResourceSlice, 0)
	for _, gens := range pools {
		var max poolEntry
		for gen, data := range gens {
			if data == nil {
				continue
			}
			if gen >= max.generation {
				max = poolEntry{generation: gen, data: data}
			}
		}
		if max.data == nil || int64(len(max.data.slices)) != max.data.expected {
			continue
		}
		valid = append(valid, max.data.slices...)
	}

	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Name < valid[j].Name
	})
	return valid
}

func sliceNodeName(slice resourcev1.ResourceSlice) (string, bool) {
	if slice.Spec.NodeName != nil && *slice.Spec.NodeName != "" {
		return *slice.Spec.NodeName, true
	}
	if slice.Spec.PerDeviceNodeSelection != nil && *slice.Spec.PerDeviceNodeSelection {
		return "", false
	}
	return "", false
}

func deviceNodeName(slice resourcev1.ResourceSlice, device resourcev1.Device) (string, bool) {
	if slice.Spec.NodeName != nil && *slice.Spec.NodeName != "" {
		return *slice.Spec.NodeName, true
	}
	if slice.Spec.PerDeviceNodeSelection != nil && *slice.Spec.PerDeviceNodeSelection {
		if device.NodeName != nil && *device.NodeName != "" {
			return *device.NodeName, true
		}
	}
	return "", false
}
