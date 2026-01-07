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

package inventory

import (
	"errors"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// Builder assembles allocatable inventory from PhysicalGPU objects.
type Builder struct {
	factory BuilderFactory
}

// NewBuilder constructs an inventory builder with optional MIG placement reader.
func NewBuilder(placements MigPlacementReader) *Builder {
	return &Builder{
		factory: NewDefaultFactory(placements),
	}
}

// Build returns inventory and a list of non-fatal errors.
func (b *Builder) Build(devices []gpuv1alpha1.PhysicalGPU) (allocatable.Inventory, []error) {
	var errs []error
	inventory := allocatable.Inventory{}
	if b.factory == nil {
		return inventory, []error{errors.New("inventory factory is not configured")}
	}

	plan := b.factory.Build(devices)
	errs = append(errs, plan.Errs...)
	if plan.Close != nil {
		defer plan.Close()
	}

	for _, pgpu := range devices {
		for _, builder := range plan.Builders {
			result, err := builder.Build(pgpu, plan.Context)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if len(result.CounterSets) > 0 {
				inventory.CounterSets = append(inventory.CounterSets, result.CounterSets...)
			}
			if len(result.Devices) > 0 {
				inventory.Devices = append(inventory.Devices, result.Devices...)
			}
		}
	}

	return inventory, errs
}
