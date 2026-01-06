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

	domainalloc "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// BuildCandidates converts resource slices into allocator candidates.
func BuildCandidates(driverName string, slices []resourcev1.ResourceSlice) []domainallocator.CandidateDevice {
	valid := filterPoolSlices(driverName, slices)
	out := make([]domainallocator.CandidateDevice, 0)

	for _, slice := range valid {
		for _, device := range slice.Spec.Devices {
			nodeName, ok := deviceNodeName(slice, device)
			if !ok {
				continue
			}
			key := domainallocator.DeviceKey{Driver: slice.Spec.Driver, Pool: slice.Spec.Pool.Name, Device: device.Name}
			out = append(out, domainallocator.CandidateDevice{
				Key:      key,
				Driver:   slice.Spec.Driver,
				Pool:     slice.Spec.Pool.Name,
				NodeName: nodeName,
				Spec:     toDeviceSpec(device),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeName == out[j].NodeName {
			return out[i].Spec.Name < out[j].Spec.Name
		}
		return out[i].NodeName < out[j].NodeName
	})

	return out
}

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

func toDeviceSpec(device resourcev1.Device) domainalloc.DeviceSpec {
	return domainalloc.DeviceSpec{
		Name:                     device.Name,
		Attributes:               toAttributes(device.Attributes),
		Capacity:                 toCapacities(device.Capacity),
		Consumes:                 toConsumes(device.ConsumesCounters),
		AllowMultipleAllocations: device.AllowMultipleAllocations != nil && *device.AllowMultipleAllocations,
	}
}

func toAttributes(attrs map[resourcev1.QualifiedName]resourcev1.DeviceAttribute) map[string]domainalloc.AttributeValue {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]domainalloc.AttributeValue, len(attrs))
	for key, val := range attrs {
		out[string(key)] = domainalloc.AttributeValue{
			String: val.StringValue,
			Int:    val.IntValue,
			Bool:   val.BoolValue,
		}
	}
	return out
}

func toCapacities(capacity map[resourcev1.QualifiedName]resourcev1.DeviceCapacity) map[string]domainalloc.CapacityValue {
	if len(capacity) == 0 {
		return nil
	}
	out := make(map[string]domainalloc.CapacityValue, len(capacity))
	for key, val := range capacity {
		out[string(key)] = capacityValueFromQuantity(val.Value, val.RequestPolicy)
	}
	return out
}

func toConsumes(consumes []resourcev1.DeviceCounterConsumption) []domainalloc.CounterConsumption {
	if len(consumes) == 0 {
		return nil
	}
	out := make([]domainalloc.CounterConsumption, 0, len(consumes))
	for _, consumption := range consumes {
		out = append(out, domainalloc.CounterConsumption{
			CounterSet: consumption.CounterSet,
			Counters:   toCounters(consumption.Counters),
		})
	}
	return out
}

func toCounters(counters map[string]resourcev1.Counter) map[string]domainalloc.CounterValue {
	if len(counters) == 0 {
		return nil
	}
	out := make(map[string]domainalloc.CounterValue, len(counters))
	for key, val := range counters {
		out[key] = counterValueFromQuantity(val.Value)
	}
	return out
}

func toCounterSet(counterSet resourcev1.CounterSet) domainalloc.CounterSet {
	return domainalloc.CounterSet{
		Name:     counterSet.Name,
		Counters: toCounters(counterSet.Counters),
	}
}
