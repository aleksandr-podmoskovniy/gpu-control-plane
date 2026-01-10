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
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func migProfileName(raw string, sliceCount uint32, memoryMiB uint64, profileID uint32) string {
	name := normalizeMigProfileName(raw)
	if name == "" {
		name = defaultMigProfileName(sliceCount, memoryMiB, profileID)
	}
	if name == "" {
		return ""
	}
	if hasProfileSuffix(name) {
		return name
	}
	suffix := migProfileSuffix(profileID)
	if suffix != "" {
		return name + suffix
	}
	return name
}

func normalizeMigProfileName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "MIG ")
	return strings.TrimSpace(name)
}

func defaultMigProfileName(sliceCount uint32, memoryMiB uint64, profileID uint32) string {
	if sliceCount == 0 || memoryMiB == 0 {
		return ""
	}
	gb := (memoryMiB + 512) / 1024
	if gb == 0 {
		return fmt.Sprintf("profile-%d", profileID)
	}
	return fmt.Sprintf("%dg.%dgb", sliceCount, gb)
}

func hasProfileSuffix(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "+me") || strings.Contains(lower, "+gfx") || strings.Contains(lower, "-me")
}

func migProfileSuffix(profileID uint32) string {
	switch profileID {
	case nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1,
		nvml.GPU_INSTANCE_PROFILE_2_SLICE_REV1:
		return "+me"
	case nvml.GPU_INSTANCE_PROFILE_1_SLICE_ALL_ME,
		nvml.GPU_INSTANCE_PROFILE_2_SLICE_ALL_ME:
		return "+me.all"
	case nvml.GPU_INSTANCE_PROFILE_1_SLICE_GFX,
		nvml.GPU_INSTANCE_PROFILE_2_SLICE_GFX,
		nvml.GPU_INSTANCE_PROFILE_4_SLICE_GFX:
		return "+gfx"
	case nvml.GPU_INSTANCE_PROFILE_1_SLICE_NO_ME,
		nvml.GPU_INSTANCE_PROFILE_2_SLICE_NO_ME:
		return "-me"
	default:
		return ""
	}
}
