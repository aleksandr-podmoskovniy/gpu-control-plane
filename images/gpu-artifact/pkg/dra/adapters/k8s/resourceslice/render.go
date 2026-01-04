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

// Renderer renders allocatable inventory into ResourceSlice slices.
type Renderer interface {
	Render(inv allocatable.Inventory) []resourcesliceapi.Slice
}

// DefaultRenderer renders counter and device slices.
type DefaultRenderer struct{}

// Render converts allocatable inventory into ResourceSlice slices.
func (DefaultRenderer) Render(inv allocatable.Inventory) []resourcesliceapi.Slice {
	var slices []resourcesliceapi.Slice
	if len(inv.CounterSets) > 0 {
		slices = append(slices, resourcesliceapi.Slice{
			SharedCounters: allocatablek8s.RenderCounterSets(inv.CounterSets),
		})
	}

	slices = append(slices, resourcesliceapi.Slice{
		Devices: allocatablek8s.RenderDevices(inv.Devices),
	})
	return slices
}

// BuildDriverResources renders inventory into DriverResources for a pool.
func BuildDriverResources(poolName string, inv allocatable.Inventory) resourcesliceapi.DriverResources {
	renderer := DefaultRenderer{}
	return resourcesliceapi.DriverResources{
		Pools: map[string]resourcesliceapi.Pool{
			poolName: {
				Slices: renderer.Render(inv),
			},
		},
	}
}
