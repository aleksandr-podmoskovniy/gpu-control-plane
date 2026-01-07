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
	"context"
	"errors"
	"sync"

	"k8s.io/dynamic-resource-allocation/resourceslice"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	k8sresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/resourceslice"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
)

// BuildResult combines inventory data with rendered driver resources.
type BuildResult struct {
	Resources resourceslice.DriverResources
	Inventory allocatable.Inventory
}

// Builder builds DriverResources for a node.
type Builder struct {
	inventory inventory.InventoryBuilder
	mu        sync.RWMutex
	features  k8sresourceslice.FeatureSet
}

// NewBuilder constructs a builder with optional MIG placement reader.
func NewBuilder(placements inventory.MigPlacementReader) *Builder {
	return &Builder{
		inventory: inventory.NewBuilder(placements),
		features:  k8sresourceslice.DefaultFeatures(),
	}
}

// Build renders driver resources for the given node and devices.
func (b *Builder) Build(_ context.Context, nodeName string, devices []gpuv1alpha1.PhysicalGPU) (BuildResult, error) {
	if b.inventory == nil {
		return BuildResult{}, errors.New("inventory builder is not configured")
	}
	poolName := PoolName(nodeName)

	inv, errs := b.inventory.Build(devices)

	b.mu.RLock()
	features := b.features
	b.mu.RUnlock()

	resources := k8sresourceslice.BuildDriverResources(poolName, inv, features)
	return BuildResult{
		Resources: resources,
		Inventory: inv,
	}, errors.Join(errs...)
}
