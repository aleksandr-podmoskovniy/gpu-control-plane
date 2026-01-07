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

	"k8s.io/apimachinery/pkg/api/resource"

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

	usedExclusive := map[DeviceKey]struct{}{}
	consumedTotals := map[DeviceKey]map[string]resource.Quantity{}
	for key, info := range allocated {
		if info.Exclusive {
			usedExclusive[key] = struct{}{}
			continue
		}
		if len(info.ConsumedCapacity) > 0 {
			consumedTotals[key] = cloneQuantities(info.ConsumedCapacity)
		}
	}

	consumedCounters := map[string]map[string]allocatable.CounterValue{}
	blockedCounterSets := map[string]struct{}{}
	if len(counterSets) > 0 {
		for key := range allocated {
			device, ok := deviceIndex[key]
			if !ok || len(device.Spec.Consumes) == 0 {
				continue
			}
			if !consumeCounters(consumedCounters, counterSets, device.Spec.Consumes) {
				blockCounterSets(blockedCounterSets, device.Spec.Consumes)
			}
		}
	}
	results := make([]domain.AllocatedDevice, 0)

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
				if info, ok := allocated[dev.Key]; ok && info.Exclusive {
					continue
				}
				for allocatedCount < req.Count {
					consumed, ok := computeConsumedCapacity(req.Capacity, dev.Spec.Capacity)
					if !ok {
						break
					}
					if !fitsCapacity(consumedTotals[dev.Key], consumed, dev.Spec.Capacity) {
						break
					}
					if len(dev.Spec.Consumes) > 0 {
						if len(counterSets) == 0 {
							break
						}
						if !consumeCounters(consumedCounters, counterSets, dev.Spec.Consumes) {
							break
						}
					}
					results = append(results, domain.AllocatedDevice{
						Request:                  req.Name,
						Driver:                   dev.Driver,
						Pool:                     dev.Pool,
						Device:                   dev.Spec.Name,
						ConsumedCapacity:         consumed,
						BindingConditions:        cloneStrings(dev.Spec.BindingConditions),
						BindingFailureConditions: cloneStrings(dev.Spec.BindingFailureConditions),
					})
					markGroupState(meta, groupState)
					consumedTotals[dev.Key] = addConsumed(consumedTotals[dev.Key], consumed, dev.Spec.Capacity)
					allocatedCount++
				}
			} else {
				if _, ok := usedExclusive[dev.Key]; ok {
					continue
				}
				if _, ok := allocated[dev.Key]; ok {
					continue
				}
				if req.Capacity != nil {
					if _, ok := computeConsumedCapacity(req.Capacity, dev.Spec.Capacity); !ok {
						continue
					}
				}
				if len(dev.Spec.Consumes) > 0 {
					if len(counterSets) == 0 {
						continue
					}
					if !consumeCounters(consumedCounters, counterSets, dev.Spec.Consumes) {
						continue
					}
				}
				results = append(results, domain.AllocatedDevice{
					Request:                  req.Name,
					Driver:                   dev.Driver,
					Pool:                     dev.Pool,
					Device:                   dev.Spec.Name,
					BindingConditions:        cloneStrings(dev.Spec.BindingConditions),
					BindingFailureConditions: cloneStrings(dev.Spec.BindingFailureConditions),
				})
				markGroupState(meta, groupState)
				usedExclusive[dev.Key] = struct{}{}
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

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
