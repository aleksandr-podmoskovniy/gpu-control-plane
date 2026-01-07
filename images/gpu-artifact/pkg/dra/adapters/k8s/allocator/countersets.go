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
	resourcev1 "k8s.io/api/resource/v1"

	domainalloc "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// BuildCounterSets converts resource slice shared counters into allocator inventory.
func BuildCounterSets(driverName string, slices []resourcev1.ResourceSlice) domainallocator.CounterSetInventory {
	valid := filterPoolSlices(driverName, slices)
	out := domainallocator.CounterSetInventory{}
	for _, slice := range valid {
		if len(slice.Spec.SharedCounters) == 0 {
			continue
		}
		nodeName, ok := sliceNodeName(slice)
		if !ok {
			continue
		}
		if out[nodeName] == nil {
			out[nodeName] = map[string]domainalloc.CounterSet{}
		}
		for _, set := range slice.Spec.SharedCounters {
			if set.Name == "" {
				continue
			}
			out[nodeName][set.Name] = toCounterSet(set)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
