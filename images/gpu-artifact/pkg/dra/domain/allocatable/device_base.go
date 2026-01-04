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

type deviceBase struct {
	uuid                     string
	deviceType               gpuv1alpha1.DeviceType
	canonicalName            string
	attributes               map[string]AttributeValue
	capacity                 map[string]CapacityValue
	consumes                 []CounterConsumption
	allowMultipleAllocations bool
}

func (d *deviceBase) UUID() string {
	return d.uuid
}

func (d *deviceBase) Type() gpuv1alpha1.DeviceType {
	return d.deviceType
}

func (d *deviceBase) CanonicalName() string {
	return d.canonicalName
}

func (d *deviceBase) Spec() DeviceSpec {
	return DeviceSpec{
		Name:                     d.canonicalName,
		Attributes:               d.attributes,
		Capacity:                 d.capacity,
		Consumes:                 d.consumes,
		AllowMultipleAllocations: d.allowMultipleAllocations,
	}
}
