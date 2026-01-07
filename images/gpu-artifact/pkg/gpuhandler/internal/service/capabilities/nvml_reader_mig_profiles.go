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

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func buildMigProfiles(dev NVMLDevice) *gpuv1alpha1.NvidiaMIGCapabilities {
	var profiles []gpuv1alpha1.NvidiaMIGProfile
	var totalSlices int32

	maxProfiles := nvml.GPU_INSTANCE_PROFILE_COUNT
	if maxProfiles < 32 {
		// NVML headers can lag behind newer profile IDs; probe a wider range.
		maxProfiles = 32
	}

	for profile := 0; profile < maxProfiles; profile++ {
		infoV3, ret := dev.GetGpuInstanceProfileInfoV3(profile)
		if ret == nvml.SUCCESS {
			name := nvmlName(infoV3.Name[:])
			mediaSuffix := mediaExtensionsSuffix(infoV3.DecoderCount, infoV3.EncoderCount, infoV3.JpegCount, infoV3.OfaCount)
			gfx := hasGfxCapability(infoV3.Capabilities)
			profiles = appendMigProfile(profiles, &totalSlices, infoV3.Id, name, infoV3.MemorySizeMB, infoV3.SliceCount, infoV3.InstanceCount, mediaSuffix, gfx)
			continue
		}

		infoV2, ret := dev.GetGpuInstanceProfileInfoV2(profile)
		if ret == nvml.SUCCESS {
			name := nvmlName(infoV2.Name[:])
			mediaSuffix := mediaExtensionsSuffix(infoV2.DecoderCount, infoV2.EncoderCount, infoV2.JpegCount, infoV2.OfaCount)
			profiles = appendMigProfile(profiles, &totalSlices, infoV2.Id, name, infoV2.MemorySizeMB, infoV2.SliceCount, infoV2.InstanceCount, mediaSuffix, false)
			continue
		}

		infoV1, ret := dev.GetGpuInstanceProfileInfo(profile)
		if ret == nvml.SUCCESS {
			mediaSuffix := mediaExtensionsSuffix(infoV1.DecoderCount, infoV1.EncoderCount, infoV1.JpegCount, infoV1.OfaCount)
			profiles = appendMigProfile(profiles, &totalSlices, infoV1.Id, "", infoV1.MemorySizeMB, infoV1.SliceCount, infoV1.InstanceCount, mediaSuffix, false)
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

func appendMigProfile(profiles []gpuv1alpha1.NvidiaMIGProfile, totalSlices *int32, id uint32, name string, memoryMiB uint64, sliceCount, instanceCount uint32, mediaSuffix string, gfx bool) []gpuv1alpha1.NvidiaMIGProfile {
	if sliceCount == 0 || memoryMiB == 0 {
		return profiles
	}

	profileName := migProfileName(name, sliceCount, memoryMiB, id, mediaSuffix, gfx)
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

func hasGfxCapability(caps uint32) bool {
	return caps&nvml.GPU_INSTANCE_PROFILE_CAPS_GFX != 0
}
