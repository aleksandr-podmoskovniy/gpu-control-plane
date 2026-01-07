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

import (
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	domain "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// RenderCapacities converts domain capacities into API capacities.
func RenderCapacities(capacity map[string]domain.CapacityValue) map[resourceapi.QualifiedName]resourceapi.DeviceCapacity {
	if len(capacity) == 0 {
		return nil
	}
	out := make(map[resourceapi.QualifiedName]resourceapi.DeviceCapacity, len(capacity))
	for key, val := range capacity {
		capacityValue := resourceapi.DeviceCapacity{
			Value: quantityFromCapacity(val.Value, val.Unit),
		}
		if val.Policy != nil {
			capacityValue.RequestPolicy = &resourceapi.CapacityRequestPolicy{
				Default: quantityPtrFromCapacity(val.Policy.Default, val.Policy.Unit),
				ValidRange: &resourceapi.CapacityRequestPolicyRange{
					Min:  quantityPtrFromCapacity(val.Policy.Min, val.Policy.Unit),
					Max:  quantityPtrFromCapacity(val.Policy.Max, val.Policy.Unit),
					Step: quantityPtrFromCapacity(val.Policy.Step, val.Policy.Unit),
				},
			}
		}
		out[resourceapi.QualifiedName(key)] = capacityValue
	}
	return out
}

func quantityFromCapacity(value int64, unit domain.CapacityUnit) resource.Quantity {
	switch unit {
	case domain.CapacityUnitMiB:
		return *resource.NewQuantity(value*1024*1024, resource.BinarySI)
	case domain.CapacityUnitPercent, domain.CapacityUnitCount:
		return *resource.NewQuantity(value, resource.DecimalSI)
	default:
		return *resource.NewQuantity(value, resource.DecimalSI)
	}
}

func quantityPtrFromCapacity(value int64, unit domain.CapacityUnit) *resource.Quantity {
	qty := quantityFromCapacity(value, unit)
	return &qty
}
