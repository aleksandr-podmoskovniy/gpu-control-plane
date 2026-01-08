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

package allocator

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func quantityValue(qty resource.Quantity, unit allocatable.CapacityUnit) int64 {
	switch unit {
	case allocatable.CapacityUnitMiB:
		return qty.Value() / (1024 * 1024)
	default:
		return qty.Value()
	}
}

func quantityFromValue(value int64, unit allocatable.CapacityUnit) resource.Quantity {
	switch unit {
	case allocatable.CapacityUnitMiB:
		return *resource.NewQuantity(value*1024*1024, resource.BinarySI)
	default:
		return *resource.NewQuantity(value, resource.DecimalSI)
	}
}
