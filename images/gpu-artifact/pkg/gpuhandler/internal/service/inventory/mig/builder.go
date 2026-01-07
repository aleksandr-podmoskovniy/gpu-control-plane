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

package mig

import (
	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	invtypes "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/types"
)

// Builder builds MIG devices and counters when supported.
type Builder struct{}

// NewBuilder returns a builder for MIG devices.
func NewBuilder() *Builder {
	return &Builder{}
}

// Build builds MIG offers and counters if supported.
func (b *Builder) Build(pgpu gpuv1alpha1.PhysicalGPU, ctx invtypes.BuildContext) (invtypes.BuildResult, error) {
	counterSets, devices, err := buildMigDevices(pgpu, ctx.MigSession)
	if err != nil {
		return invtypes.BuildResult{}, err
	}
	return invtypes.BuildResult{Devices: devices, CounterSets: counterSets}, nil
}
