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

import "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"

func counterSetBlocked(consumes []allocatable.CounterConsumption, blocked map[string]struct{}) bool {
	if len(consumes) == 0 {
		return false
	}
	for _, consumption := range consumes {
		if consumption.CounterSet == "" {
			return true
		}
		if _, ok := blocked[consumption.CounterSet]; ok {
			return true
		}
	}
	return false
}

func blockCounterSets(blocked map[string]struct{}, consumes []allocatable.CounterConsumption) {
	if len(consumes) == 0 {
		return
	}
	for _, consumption := range consumes {
		if consumption.CounterSet == "" {
			continue
		}
		blocked[consumption.CounterSet] = struct{}{}
	}
}

func consumeCounters(consumed map[string]map[string]allocatable.CounterValue, available map[string]allocatable.CounterSet, consumes []allocatable.CounterConsumption) bool {
	if len(consumes) == 0 {
		return true
	}
	for _, consumption := range consumes {
		if consumption.CounterSet == "" {
			return false
		}
		set, ok := available[consumption.CounterSet]
		if !ok {
			return false
		}
		if !fitsCounters(set.Counters, consumed[consumption.CounterSet], consumption.Counters) {
			return false
		}
	}
	for _, consumption := range consumes {
		set := available[consumption.CounterSet]
		consumed[consumption.CounterSet] = addCounters(consumed[consumption.CounterSet], consumption.Counters, set.Counters)
	}
	return true
}

func fitsCounters(available map[string]allocatable.CounterValue, current map[string]allocatable.CounterValue, add map[string]allocatable.CounterValue) bool {
	if len(add) == 0 {
		return true
	}
	if len(available) == 0 {
		return false
	}
	for name, addVal := range add {
		avail, ok := available[name]
		if !ok {
			return false
		}
		if addVal.Unit != avail.Unit {
			return false
		}
		total := addVal.Value
		if currentVal, ok := current[name]; ok {
			if currentVal.Unit != avail.Unit {
				return false
			}
			total += currentVal.Value
		}
		if total > avail.Value {
			return false
		}
	}
	return true
}

func addCounters(current map[string]allocatable.CounterValue, add map[string]allocatable.CounterValue, available map[string]allocatable.CounterValue) map[string]allocatable.CounterValue {
	if len(add) == 0 {
		return current
	}
	if current == nil {
		current = map[string]allocatable.CounterValue{}
	}
	for name, addVal := range add {
		avail := available[name]
		total := addVal.Value
		if currentVal, ok := current[name]; ok {
			total += currentVal.Value
		}
		current[name] = allocatable.CounterValue{Value: total, Unit: avail.Unit}
	}
	return current
}
