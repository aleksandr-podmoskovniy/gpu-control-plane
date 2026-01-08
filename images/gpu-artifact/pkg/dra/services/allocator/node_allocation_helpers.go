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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

type nodeAllocationState struct {
	allocated        map[DeviceKey]AllocatedDeviceInfo
	counterSets      map[string]allocatable.CounterSet
	consumedTotals   map[DeviceKey]map[string]resource.Quantity
	consumedCounters map[string]map[string]allocatable.CounterValue
	usedExclusive    map[DeviceKey]struct{}
}

func (s *nodeAllocationState) allocateShared(req Request, dev CandidateDevice, meta deviceMeta, groupState map[string]groupState, remaining int64, results *[]domain.AllocatedDevice) int64 {
	if remaining <= 0 {
		return 0
	}
	if info, ok := s.allocated[dev.Key]; ok && info.Exclusive {
		return 0
	}

	var allocatedCount int64
	for allocatedCount < remaining {
		consumed, ok := computeConsumedCapacity(req.Capacity, dev.Spec.Capacity)
		if !ok {
			break
		}
		if !fitsCapacity(s.consumedTotals[dev.Key], consumed, dev.Spec.Capacity) {
			break
		}
		if len(dev.Spec.Consumes) > 0 {
			if len(s.counterSets) == 0 {
				break
			}
			if !consumeCounters(s.consumedCounters, s.counterSets, dev.Spec.Consumes) {
				break
			}
		}
		*results = append(*results, domain.AllocatedDevice{
			Request:                  req.Name,
			Driver:                   dev.Driver,
			Pool:                     dev.Pool,
			Device:                   dev.Spec.Name,
			ConsumedCapacity:         consumed,
			BindingConditions:        cloneStrings(dev.Spec.BindingConditions),
			BindingFailureConditions: cloneStrings(dev.Spec.BindingFailureConditions),
		})
		markGroupState(meta, groupState)
		s.consumedTotals[dev.Key] = addConsumed(s.consumedTotals[dev.Key], consumed, dev.Spec.Capacity)
		allocatedCount++
	}

	return allocatedCount
}

func (s *nodeAllocationState) allocateExclusive(req Request, dev CandidateDevice, meta deviceMeta, groupState map[string]groupState, results *[]domain.AllocatedDevice) bool {
	if _, ok := s.usedExclusive[dev.Key]; ok {
		return false
	}
	if _, ok := s.allocated[dev.Key]; ok {
		return false
	}
	if req.Capacity != nil {
		if _, ok := computeConsumedCapacity(req.Capacity, dev.Spec.Capacity); !ok {
			return false
		}
	}
	if len(dev.Spec.Consumes) > 0 {
		if len(s.counterSets) == 0 {
			return false
		}
		if !consumeCounters(s.consumedCounters, s.counterSets, dev.Spec.Consumes) {
			return false
		}
	}
	*results = append(*results, domain.AllocatedDevice{
		Request:                  req.Name,
		Driver:                   dev.Driver,
		Pool:                     dev.Pool,
		Device:                   dev.Spec.Name,
		BindingConditions:        cloneStrings(dev.Spec.BindingConditions),
		BindingFailureConditions: cloneStrings(dev.Spec.BindingFailureConditions),
	})
	markGroupState(meta, groupState)
	s.usedExclusive[dev.Key] = struct{}{}
	return true
}
