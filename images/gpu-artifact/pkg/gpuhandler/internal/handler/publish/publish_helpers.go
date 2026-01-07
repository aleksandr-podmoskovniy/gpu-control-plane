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

package publish

import (
	"strconv"
	"strings"

	resourceapi "k8s.io/api/resource/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func pciAddress(pgpu gpuv1alpha1.PhysicalGPU) string {
	if pgpu.Status.PCIInfo == nil {
		return ""
	}
	return pgpu.Status.PCIInfo.Address
}

func migTotalSlices(pgpu gpuv1alpha1.PhysicalGPU) int32 {
	if pgpu.Status.Capabilities == nil || pgpu.Status.Capabilities.Nvidia == nil || pgpu.Status.Capabilities.Nvidia.MIG == nil {
		return 0
	}
	return pgpu.Status.Capabilities.Nvidia.MIG.TotalSlices
}

func maxMemorySliceIndex(consumes []resourceapi.DeviceCounterConsumption) (int32, bool) {
	const prefix = "memory-slice-"
	maxIdx := int32(-1)
	for _, consumption := range consumes {
		for name := range consumption.Counters {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			raw := strings.TrimPrefix(name, prefix)
			if raw == "" {
				continue
			}
			idx, err := strconv.Atoi(raw)
			if err != nil {
				continue
			}
			if int32(idx) > maxIdx {
				maxIdx = int32(idx)
			}
		}
	}
	if maxIdx < 0 {
		return 0, false
	}
	return maxIdx, true
}
