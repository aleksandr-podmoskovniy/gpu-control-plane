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
	"context"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func allocateOnNode(ctx context.Context, nodeName string, devices []CandidateDevice, requests []Request, allocated map[DeviceKey]AllocatedDeviceInfo, counterSets map[string]allocatable.CounterSet) (*domain.AllocationResult, bool, error) {
	if len(devices) == 0 {
		return nil, false, nil
	}

	metaByKey := buildDeviceMeta(devices)
	groupState := buildGroupState(metaByKey, allocated)
	deviceIndex := indexDevices(devices)

	usedExclusive, consumedTotals := buildCapacityState(allocated)
	consumedCounters, blockedCounterSets := buildCounterState(allocated, deviceIndex, counterSets)
	results := make([]domain.AllocatedDevice, 0)
	state := &nodeAllocationState{
		allocated:        allocated,
		counterSets:      counterSets,
		consumedTotals:   consumedTotals,
		consumedCounters: consumedCounters,
		usedExclusive:    usedExclusive,
	}

	for _, req := range requests {
		var allocatedCount int64
		for _, dev := range devices {
			meta := metaByKey[dev.Key]
			if counterSetBlocked(dev.Spec.Consumes, blockedCounterSets) {
				continue
			}
			if conflictsWithGroup(meta, groupState) {
				continue
			}
			match, err := matchesDevice(ctx, dev.Driver, dev.Spec, req.Selectors)
			if err != nil {
				return nil, false, err
			}
			if !match {
				continue
			}

			if dev.Spec.AllowMultipleAllocations {
				allocatedCount += state.allocateShared(req, dev, meta, groupState, req.Count-allocatedCount, &results)
			} else if state.allocateExclusive(req, dev, meta, groupState, &results) {
				allocatedCount++
			}
			if allocatedCount >= req.Count {
				break
			}
		}
		if allocatedCount < req.Count {
			return nil, false, nil
		}
	}

	return &domain.AllocationResult{
		NodeName: nodeName,
		Devices:  results,
		NodeSelector: &domain.NodeSelector{
			NodeName: nodeName,
		},
	}, true, nil
}
