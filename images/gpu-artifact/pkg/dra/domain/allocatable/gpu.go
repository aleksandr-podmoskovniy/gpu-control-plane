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

package allocatable

import gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"

// GPUDevice represents a physical GPU offer.
type GPUDevice struct {
	deviceBase
}

// NewGPUDevice constructs a physical GPU offer.
func NewGPUDevice(name, uuid string, attrs map[string]AttributeValue, capacity map[string]CapacityValue, consumes []CounterConsumption, allowMultiple bool) *GPUDevice {
	return &GPUDevice{
		deviceBase: deviceBase{
			uuid:                     uuid,
			deviceType:               gpuv1alpha1.DeviceTypePhysical,
			canonicalName:            name,
			attributes:               attrs,
			capacity:                 capacity,
			consumes:                 consumes,
			allowMultipleAllocations: allowMultiple,
			bindingConditions:        []string{DeviceConditionReady},
			bindingFailureConditions: []string{DeviceConditionReady},
		},
	}
}
