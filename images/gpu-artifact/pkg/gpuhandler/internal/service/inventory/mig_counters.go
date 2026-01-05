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
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func buildMigCounters(totalMemoryMiB int64, totalSlices int32) map[string]allocatable.CounterValue {
	counters := map[string]allocatable.CounterValue{
		"memory": {Value: totalMemoryMiB, Unit: allocatable.CounterUnitMiB},
	}
	for i := int32(0); i < totalSlices; i++ {
		counters[fmt.Sprintf("memory-slice-%d", i)] = allocatable.CounterValue{Value: 1, Unit: allocatable.CounterUnitCount}
	}
	return counters
}

func buildMigConsumes(memoryMiB int32, placement MigPlacement) map[string]allocatable.CounterValue {
	counters := map[string]allocatable.CounterValue{
		"memory": {Value: int64(memoryMiB), Unit: allocatable.CounterUnitMiB},
	}
	for i := placement.Start; i < placement.Start+placement.Size; i++ {
		counters[fmt.Sprintf("memory-slice-%d", i)] = allocatable.CounterValue{Value: 1, Unit: allocatable.CounterUnitCount}
	}
	return counters
}
