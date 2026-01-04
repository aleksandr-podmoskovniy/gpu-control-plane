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
	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

// MigDeviceBuilder builds MIG devices and counters when supported.
type MigDeviceBuilder struct{}

// NewMigDeviceBuilder returns a builder for MIG devices.
func NewMigDeviceBuilder() *MigDeviceBuilder {
	return &MigDeviceBuilder{}
}

// Build builds MIG offers and counters if supported.
func (b *MigDeviceBuilder) Build(pgpu gpuv1alpha1.PhysicalGPU, ctx BuildContext) (BuildResult, error) {
	counterSets, devices, err := buildMigDevices(pgpu, ctx.MigSession)
	if err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Devices: devices, CounterSets: counterSets}, nil
}
