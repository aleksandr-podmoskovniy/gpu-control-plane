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

package resourceslice

import (
	resourcesliceapi "k8s.io/dynamic-resource-allocation/resourceslice"

	allocatablek8s "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func renderSlicePlan(plan slicePlan, renderOpts allocatablek8s.DeviceRenderOptions, layout SharedCountersLayout) []resourcesliceapi.Slice {
	slices := make([]resourcesliceapi.Slice, 0, len(plan.groupKeys)*2+1)
	for _, key := range plan.groupKeys {
		group := plan.groups[key]
		if layout == SharedCountersSeparate {
			if len(group.counterSets) > 0 {
				slices = append(slices, resourcesliceapi.Slice{
					SharedCounters: allocatablek8s.RenderCounterSets(group.counterSets),
				})
			}
			if len(group.devicesNoConsumption) > 0 || len(group.devicesWithCounters) > 0 {
				devices := append(allocatable.DeviceList{}, group.devicesNoConsumption...)
				devices = append(devices, group.devicesWithCounters...)
				slices = append(slices, resourcesliceapi.Slice{
					Devices: allocatablek8s.RenderDevicesWithOptions(devices, renderOpts),
				})
			}
			continue
		}

		if len(group.devicesNoConsumption) > 0 {
			slices = append(slices, resourcesliceapi.Slice{
				Devices: allocatablek8s.RenderDevicesWithOptions(group.devicesNoConsumption, renderOpts),
			})
		}
		if len(group.counterSets) > 0 || len(group.devicesWithCounters) > 0 {
			slices = append(slices, resourcesliceapi.Slice{
				SharedCounters: allocatablek8s.RenderCounterSets(group.counterSets),
				Devices:        allocatablek8s.RenderDevicesWithOptions(group.devicesWithCounters, renderOpts),
			})
		}
	}

	if len(plan.standalone) > 0 {
		slices = append(slices, resourcesliceapi.Slice{
			Devices: allocatablek8s.RenderDevicesWithOptions(plan.standalone, renderOpts),
		})
	}

	if len(slices) == 0 {
		return []resourcesliceapi.Slice{{}}
	}
	return slices
}
