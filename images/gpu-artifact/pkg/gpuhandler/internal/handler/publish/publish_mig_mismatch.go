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
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func (h *PublishResourcesHandler) recordMigPlacementMismatch(ctx context.Context, st state.State, resources resourceslice.DriverResources) {
	if h.recorder == nil {
		return
	}
	ready := st.Ready()
	if len(ready) == 0 {
		return
	}

	totalByPCI := map[string]int32{}
	gpuByPCI := map[string]*gpuv1alpha1.PhysicalGPU{}
	for i := range ready {
		pci := strings.TrimSpace(pciAddress(ready[i]))
		if pci == "" {
			continue
		}
		totalSlices := migTotalSlices(ready[i])
		if totalSlices == 0 {
			continue
		}
		totalByPCI[pci] = totalSlices
		gpuByPCI[pci] = &ready[i]
	}
	if len(totalByPCI) == 0 {
		return
	}

	maxByPCI := map[string]int32{}
	for _, pool := range resources.Pools {
		for _, slice := range pool.Slices {
			for _, dev := range slice.Devices {
				if !deviceIsMIG(dev) {
					continue
				}
				pci := devicePCI(dev)
				if pci == "" {
					continue
				}
				totalSlices, ok := totalByPCI[pci]
				if !ok {
					continue
				}
				maxIdx, ok := maxMemorySliceIndex(dev.ConsumesCounters)
				if !ok || maxIdx < totalSlices {
					continue
				}
				if current, ok := maxByPCI[pci]; !ok || maxIdx > current {
					maxByPCI[pci] = maxIdx
				}
			}
		}
	}
	if len(maxByPCI) == 0 {
		return
	}

	log := logger.FromContext(ctx).With("node", st.NodeName())
	for pci, maxIdx := range maxByPCI {
		pgpu := gpuByPCI[pci]
		if pgpu == nil {
			continue
		}
		totalSlices := totalByPCI[pci]
		msg := fmt.Sprintf("MIG placements require memory-slice-%d but totalSlices=%d; check driver/NVML placement data", maxIdx, totalSlices)
		h.recorder.WithLogging(log.With("pci", pci, "totalSlices", totalSlices, "maxSlice", maxIdx)).Event(
			pgpu,
			corev1.EventTypeWarning,
			reasonMigPlacementMismatch,
			msg,
		)
	}
}
