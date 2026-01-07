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

package allocatable

import (
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	domain "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// RenderCounterSets converts domain counter sets into API counter sets.
func RenderCounterSets(counterSets []domain.CounterSet) []resourceapi.CounterSet {
	if len(counterSets) == 0 {
		return nil
	}
	out := make([]resourceapi.CounterSet, 0, len(counterSets))
	for _, set := range counterSets {
		out = append(out, resourceapi.CounterSet{
			Name:     set.Name,
			Counters: renderCounters(set.Counters),
		})
	}
	return out
}

func renderConsumes(consumes []domain.CounterConsumption) []resourceapi.DeviceCounterConsumption {
	out := make([]resourceapi.DeviceCounterConsumption, 0, len(consumes))
	for _, consumption := range consumes {
		out = append(out, resourceapi.DeviceCounterConsumption{
			CounterSet: consumption.CounterSet,
			Counters:   renderCounters(consumption.Counters),
		})
	}
	return out
}

func renderCounters(values map[string]domain.CounterValue) map[string]resourceapi.Counter {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]resourceapi.Counter, len(values))
	for key, val := range values {
		out[key] = resourceapi.Counter{
			Value: quantityFromCounter(val.Value, val.Unit),
		}
	}
	return out
}

func quantityFromCounter(value int64, unit domain.CounterUnit) resource.Quantity {
	switch unit {
	case domain.CounterUnitMiB:
		return *resource.NewQuantity(value*1024*1024, resource.BinarySI)
	case domain.CounterUnitCount:
		return *resource.NewQuantity(value, resource.DecimalSI)
	default:
		return *resource.NewQuantity(value, resource.DecimalSI)
	}
}
