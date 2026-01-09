//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package capabilities

import gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"

type migProfileCounters struct {
	Multiprocessors int32
	CopyEngines     int32
	Decoders        int32
	Encoders        int32
	JpegEngines     int32
	OfaEngines      int32
}

func appendMigProfile(
	profiles []gpuv1alpha1.NvidiaMIGProfile,
	totalSlices *int32,
	id uint32,
	name string,
	memoryMiB uint64,
	sliceCount, instanceCount uint32,
	counters migProfileCounters,
) []gpuv1alpha1.NvidiaMIGProfile {
	if sliceCount == 0 || memoryMiB == 0 {
		return profiles
	}

	profileName := migProfileName(name, sliceCount, memoryMiB, id)
	if profileName == "" {
		return profiles
	}

	profiles = append(profiles, gpuv1alpha1.NvidiaMIGProfile{
		ProfileID:       int32(id),
		Name:            profileName,
		MemoryMiB:       int32(memoryMiB),
		SliceCount:      int32(sliceCount),
		MaxInstances:    int32(instanceCount),
		Multiprocessors: counters.Multiprocessors,
		CopyEngines:     counters.CopyEngines,
		Decoders:        counters.Decoders,
		Encoders:        counters.Encoders,
		JpegEngines:     counters.JpegEngines,
		OfaEngines:      counters.OfaEngines,
	})

	candidate := int32(sliceCount) * int32(instanceCount)
	if candidate > *totalSlices {
		*totalSlices = candidate
	}
	return profiles
}
