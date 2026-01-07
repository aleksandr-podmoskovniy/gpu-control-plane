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
	invmig "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/mig"
	invphysical "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/physical"
)

// BuilderFactory decides which device builders to use for a set of GPUs.
type BuilderFactory interface {
	Build(devices []gpuv1alpha1.PhysicalGPU) Plan
}

// DefaultFactory selects builders based on GPU capabilities.
type DefaultFactory struct {
	placements MigPlacementReader
}

// NewDefaultFactory creates a factory with the provided MIG placements reader.
func NewDefaultFactory(placements MigPlacementReader) *DefaultFactory {
	return &DefaultFactory{placements: placements}
}

// Build decides which builders to run based on GPU capabilities.
func (f *DefaultFactory) Build(devices []gpuv1alpha1.PhysicalGPU) Plan {
	plan := Plan{
		Builders: []DeviceBuilder{
			invphysical.NewBuilder(),
		},
	}

	if !invmig.AnySupported(devices) {
		return plan
	}
	if f.placements == nil {
		plan.Errs = append(plan.Errs, errors.New("mig placements reader is not configured"))
		return plan
	}
	session, err := f.placements.Open()
	if err != nil {
		plan.Errs = append(plan.Errs, err)
		return plan
	}

	plan.Context = BuildContext{MigSession: session}
	plan.Builders = append(plan.Builders, invmig.NewBuilder())
	plan.Close = session.Close
	return plan
}
