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

package service

import (
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func readMigMode(dev NVMLDevice) *gpuv1alpha1.NvidiaMIGState {
	current, _, ret := dev.GetMigMode()
	if ret != nvml.SUCCESS {
		return nil
	}
	return &gpuv1alpha1.NvidiaMIGState{Mode: migModeFromNVML(current)}
}

func readMigCapabilities(dev NVMLDevice) (*bool, *gpuv1alpha1.NvidiaMIGCapabilities) {
	_, _, ret := dev.GetMigMode()
	switch ret {
	case nvml.SUCCESS:
		supported := true
		return boolPtr(supported), buildMigProfiles(dev)
	case nvml.ERROR_NOT_SUPPORTED:
		supported := false
		return boolPtr(supported), nil
	default:
		return nil, nil
	}
}

func buildMigProfiles(dev NVMLDevice) *gpuv1alpha1.NvidiaMIGCapabilities {
	var profiles []gpuv1alpha1.NvidiaMIGProfile
	var totalSlices int32

	for profile := 0; profile < nvml.GPU_INSTANCE_PROFILE_COUNT; profile++ {
		infoV3, ret := dev.GetGpuInstanceProfileInfoV3(profile)
		if ret == nvml.SUCCESS {
			name := nvmlName(infoV3.Name[:])
			profiles = appendMigProfile(profiles, &totalSlices, infoV3.Id, name, infoV3.MemorySizeMB, infoV3.SliceCount, infoV3.InstanceCount)
			continue
		}

		infoV2, ret := dev.GetGpuInstanceProfileInfoV2(profile)
		if ret == nvml.SUCCESS {
			name := nvmlName(infoV2.Name[:])
			profiles = appendMigProfile(profiles, &totalSlices, infoV2.Id, name, infoV2.MemorySizeMB, infoV2.SliceCount, infoV2.InstanceCount)
			continue
		}

		infoV1, ret := dev.GetGpuInstanceProfileInfo(profile)
		if ret == nvml.SUCCESS {
			profiles = appendMigProfile(profiles, &totalSlices, infoV1.Id, "", infoV1.MemorySizeMB, infoV1.SliceCount, infoV1.InstanceCount)
		}
	}

	if len(profiles) == 0 {
		return nil
	}
	return &gpuv1alpha1.NvidiaMIGCapabilities{
		TotalSlices: totalSlices,
		Profiles:    profiles,
	}
}

func appendMigProfile(profiles []gpuv1alpha1.NvidiaMIGProfile, totalSlices *int32, id uint32, name string, memoryMiB uint64, sliceCount, instanceCount uint32) []gpuv1alpha1.NvidiaMIGProfile {
	if sliceCount == 0 || memoryMiB == 0 {
		return profiles
	}

	profileName := migProfileName(name, sliceCount, memoryMiB, id)
	if profileName == "" {
		return profiles
	}

	profiles = append(profiles, gpuv1alpha1.NvidiaMIGProfile{
		ProfileID:    int32(id),
		Name:         profileName,
		MemoryMiB:    int32(memoryMiB),
		SliceCount:   int32(sliceCount),
		MaxInstances: int32(instanceCount),
	})

	candidate := int32(sliceCount) * int32(instanceCount)
	if candidate > *totalSlices {
		*totalSlices = candidate
	}
	return profiles
}

func migProfileName(raw string, sliceCount uint32, memoryMiB uint64, profileID uint32) string {
	name := strings.TrimSpace(raw)
	if name != "" {
		return name
	}
	if sliceCount == 0 || memoryMiB == 0 {
		return ""
	}

	gb := (memoryMiB + 512) / 1024
	if gb == 0 {
		return fmt.Sprintf("profile-%d", profileID)
	}
	return fmt.Sprintf("%dg.%dgb", sliceCount, gb)
}
