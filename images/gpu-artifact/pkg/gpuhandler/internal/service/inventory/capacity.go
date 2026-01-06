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

import "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"

func memoryCapacityMiB(memoryMiB int64) allocatable.CapacityValue {
	return allocatable.CapacityValue{
		Value: memoryMiB,
		Unit:  allocatable.CapacityUnitMiB,
	}
}

func shareableMemoryCapacityMiB(memoryMiB int64) allocatable.CapacityValue {
	return allocatable.CapacityValue{
		Value: memoryMiB,
		Unit:  allocatable.CapacityUnitMiB,
		Policy: &allocatable.CapacityPolicy{
			Default: 0,
			Min:     0,
			Max:     memoryMiB,
			Step:    1,
			Unit:    allocatable.CapacityUnitMiB,
		},
	}
}

func sharePercentCapacity() allocatable.CapacityValue {
	return allocatable.CapacityValue{
		Value: 100,
		Unit:  allocatable.CapacityUnitPercent,
		Policy: &allocatable.CapacityPolicy{
			Default: 100,
			Min:     1,
			Max:     100,
			Step:    1,
			Unit:    allocatable.CapacityUnitPercent,
		},
	}
}
