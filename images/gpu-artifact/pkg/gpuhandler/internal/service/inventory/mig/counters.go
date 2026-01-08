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
	"fmt"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	invtypes "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/types"
)

const (
	counterMemory         = "memory"
	counterMultiprocessor = "multiprocessors"
	counterCopyEngines    = "copy-engines"
	counterDecoders       = "decoders"
	counterEncoders       = "encoders"
	counterJpegEngines    = "jpeg-engines"
	counterOfaEngines     = "ofa-engines"
)

type migCountersTotal struct {
	MemoryMiB       int64
	TotalSlices     int32
	Multiprocessors int32
	CopyEngines     int32
	Decoders        int32
	Encoders        int32
	JpegEngines     int32
	OfaEngines      int32
}

func totalsFromProfiles(memoryMiB int64, totalSlices int32, profiles []gpuv1alpha1.NvidiaMIGProfile) migCountersTotal {
	totals := migCountersTotal{
		MemoryMiB:   memoryMiB,
		TotalSlices: totalSlices,
	}
	for _, profile := range profiles {
		max := profile.MaxInstances
		if max < 1 {
			max = 1
		}
		totals.Multiprocessors = maxCounterTotal(totals.Multiprocessors, profile.Multiprocessors, max)
		totals.CopyEngines = maxCounterTotal(totals.CopyEngines, profile.CopyEngines, max)
		totals.Decoders = maxCounterTotal(totals.Decoders, profile.Decoders, max)
		totals.Encoders = maxCounterTotal(totals.Encoders, profile.Encoders, max)
		totals.JpegEngines = maxCounterTotal(totals.JpegEngines, profile.JpegEngines, max)
		totals.OfaEngines = maxCounterTotal(totals.OfaEngines, profile.OfaEngines, max)
	}
	return totals
}

func maxCounterTotal(current, perProfile, instances int32) int32 {
	if perProfile <= 0 {
		return current
	}
	total := perProfile * instances
	if total > current {
		return total
	}
	return current
}

func buildMigCounters(totals migCountersTotal) map[string]allocatable.CounterValue {
	counters := map[string]allocatable.CounterValue{
		counterMemory: {Value: totals.MemoryMiB, Unit: allocatable.CounterUnitMiB},
	}
	for i := int32(0); i < totals.TotalSlices; i++ {
		counters[fmt.Sprintf("memory-slice-%d", i)] = allocatable.CounterValue{Value: 1, Unit: allocatable.CounterUnitCount}
	}
	addCounterIfPositive(counters, counterMultiprocessor, totals.Multiprocessors)
	addCounterIfPositive(counters, counterCopyEngines, totals.CopyEngines)
	addCounterIfPositive(counters, counterDecoders, totals.Decoders)
	addCounterIfPositive(counters, counterEncoders, totals.Encoders)
	addCounterIfPositive(counters, counterJpegEngines, totals.JpegEngines)
	addCounterIfPositive(counters, counterOfaEngines, totals.OfaEngines)
	return counters
}

func buildMigConsumes(profile gpuv1alpha1.NvidiaMIGProfile, placement invtypes.MigPlacement) map[string]allocatable.CounterValue {
	counters := map[string]allocatable.CounterValue{
		counterMemory: {Value: int64(profile.MemoryMiB), Unit: allocatable.CounterUnitMiB},
	}
	for i := placement.Start; i < placement.Start+placement.Size; i++ {
		counters[fmt.Sprintf("memory-slice-%d", i)] = allocatable.CounterValue{Value: 1, Unit: allocatable.CounterUnitCount}
	}
	addCounterIfPositive(counters, counterMultiprocessor, profile.Multiprocessors)
	addCounterIfPositive(counters, counterCopyEngines, profile.CopyEngines)
	addCounterIfPositive(counters, counterDecoders, profile.Decoders)
	addCounterIfPositive(counters, counterEncoders, profile.Encoders)
	addCounterIfPositive(counters, counterJpegEngines, profile.JpegEngines)
	addCounterIfPositive(counters, counterOfaEngines, profile.OfaEngines)
	return counters
}

func addCounterIfPositive(target map[string]allocatable.CounterValue, name string, value int32) {
	if value <= 0 {
		return
	}
	target[name] = allocatable.CounterValue{Value: int64(value), Unit: allocatable.CounterUnitCount}
}
