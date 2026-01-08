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

package mig

import "github.com/NVIDIA/go-nvml/pkg/nvml"

func computeProfileForSlices(sliceCount uint32, gpuInstance nvml.GpuInstance) (nvml.ComputeInstanceProfileInfo, nvml.Return) {
	profile, ok := computeProfileBySliceCount(sliceCount)
	if !ok {
		return nvml.ComputeInstanceProfileInfo{}, nvml.ERROR_INVALID_ARGUMENT
	}
	if gpuInstance == nil {
		return nvml.ComputeInstanceProfileInfo{Id: uint32(profile)}, nvml.SUCCESS
	}
	return gpuInstance.GetComputeInstanceProfileInfo(profile, nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_SHARED)
}

func computeProfileBySliceCount(sliceCount uint32) (int, bool) {
	switch sliceCount {
	case 1:
		return nvml.COMPUTE_INSTANCE_PROFILE_1_SLICE, true
	case 2:
		return nvml.COMPUTE_INSTANCE_PROFILE_2_SLICE, true
	case 3:
		return nvml.COMPUTE_INSTANCE_PROFILE_3_SLICE, true
	case 4:
		return nvml.COMPUTE_INSTANCE_PROFILE_4_SLICE, true
	case 6:
		return nvml.COMPUTE_INSTANCE_PROFILE_6_SLICE, true
	case 7:
		return nvml.COMPUTE_INSTANCE_PROFILE_7_SLICE, true
	case 8:
		return nvml.COMPUTE_INSTANCE_PROFILE_8_SLICE, true
	default:
		return 0, false
	}
}

func selectComputePlacement(info nvml.ComputeInstanceProfileInfo) nvml.ComputeInstancePlacement {
	return nvml.ComputeInstancePlacement{Start: 0, Size: info.SliceCount}
}
