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
)

func toDeviceSpec(device resourcev1.Device) domainalloc.DeviceSpec {
	return domainalloc.DeviceSpec{
		Name:                     device.Name,
		Attributes:               toAttributes(device.Attributes),
		Capacity:                 toCapacities(device.Capacity),
		Consumes:                 toConsumes(device.ConsumesCounters),
		AllowMultipleAllocations: device.AllowMultipleAllocations != nil && *device.AllowMultipleAllocations,
		BindingConditions:        cloneStrings(device.BindingConditions),
		BindingFailureConditions: cloneStrings(device.BindingFailureConditions),
	}
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
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
