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
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func buildCapacityState(allocated map[DeviceKey]AllocatedDeviceInfo) (map[DeviceKey]struct{}, map[DeviceKey]map[string]resource.Quantity) {
	usedExclusive := map[DeviceKey]struct{}{}
	consumedTotals := map[DeviceKey]map[string]resource.Quantity{}
	if len(allocated) == 0 {
		return usedExclusive, consumedTotals
	}
	for key, info := range allocated {
		if info.Exclusive {
			usedExclusive[key] = struct{}{}
		}
		if len(info.ConsumedCapacity) > 0 {
			consumedTotals[key] = cloneQuantities(info.ConsumedCapacity)
		}
	}
	return usedExclusive, consumedTotals
}

func buildCounterState(allocated map[DeviceKey]AllocatedDeviceInfo, deviceIndex map[DeviceKey]CandidateDevice, counterSets map[string]allocatable.CounterSet) (map[string]map[string]allocatable.CounterValue, map[string]struct{}) {
	consumedCounters := map[string]map[string]allocatable.CounterValue{}
	blockedCounterSets := map[string]struct{}{}
	if len(allocated) == 0 {
		return consumedCounters, blockedCounterSets
	}
	for key := range allocated {
		device, ok := deviceIndex[key]
		if !ok {
			continue
		}
		consumes := device.Spec.Consumes
		if len(consumes) == 0 {
			continue
		}
		if len(counterSets) == 0 {
			blockCounterSets(blockedCounterSets, consumes)
			continue
		}
		if !consumeCounters(consumedCounters, counterSets, consumes) {
			blockCounterSets(blockedCounterSets, consumes)
		}
	}
	return consumedCounters, blockedCounterSets
}
