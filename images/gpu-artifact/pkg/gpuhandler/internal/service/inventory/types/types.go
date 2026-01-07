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

package types

import (
	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// BuildContext contains shared context for device builders.
type BuildContext struct {
	MigSession MigPlacementSession
}

// BuildResult groups devices and counters produced by a builder.
type BuildResult struct {
	Devices     allocatable.DeviceList
	CounterSets []allocatable.CounterSet
}

// DeviceBuilder converts a PhysicalGPU into allocatable devices/counters.
type DeviceBuilder interface {
	Build(pgpu gpuv1alpha1.PhysicalGPU, ctx BuildContext) (BuildResult, error)
}

// InventoryBuilder assembles allocatable inventory from PhysicalGPU objects.
type InventoryBuilder interface {
	Build(devices []gpuv1alpha1.PhysicalGPU) (allocatable.Inventory, []error)
}
