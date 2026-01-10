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

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	allocatablek8s "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// Renderer renders allocatable inventory into ResourceSlice slices.
type Renderer interface {
	Render(inv allocatable.Inventory) []resourcesliceapi.Slice
}

// DefaultRenderer renders counter and device slices.
type DefaultRenderer struct {
	Features FeatureSet
}

// Render converts allocatable inventory into ResourceSlice slices.
func (r DefaultRenderer) Render(inv allocatable.Inventory) []resourcesliceapi.Slice {
	inv = filterInventory(inv, r.Features)
	renderOpts := allocatablek8s.DeviceRenderOptions{
		IncludeCapacity:          r.Features.ConsumableCapacity,
		IncludeMultiAllocations:  r.Features.ConsumableCapacity,
		IncludeBindingConditions: r.Features.BindingConditions,
	}

	if len(inv.CounterSets) == 0 {
		return []resourcesliceapi.Slice{{
			Devices: allocatablek8s.RenderDevicesWithOptions(inv.Devices, renderOpts),
		}}
	}

	plan := buildSlicePlan(inv)
	return renderSlicePlan(plan, renderOpts, r.Features.SharedCountersLayout)
}

// BuildDriverResources renders inventory into DriverResources for a pool.
func BuildDriverResources(poolName string, inv allocatable.Inventory, features FeatureSet) resourcesliceapi.DriverResources {
	renderer := DefaultRenderer{Features: features}
	return resourcesliceapi.DriverResources{
		Pools: map[string]resourcesliceapi.Pool{
			poolName: {
				Slices: renderer.Render(inv),
			},
		},
	}
}

func filterInventory(inv allocatable.Inventory, features FeatureSet) allocatable.Inventory {
	if !features.PartitionableDevices {
		inv.CounterSets = nil
		inv.Devices = inv.Devices.FilterByType(gpuv1alpha1.DeviceTypePhysical)
	}
	return inv
}
