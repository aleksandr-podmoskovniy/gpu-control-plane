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
	invcommon "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/common"
	invtypes "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/types"
)

func buildMigDevices(pgpu gpuv1alpha1.PhysicalGPU, session invtypes.MigPlacementSession) ([]allocatable.CounterSet, allocatable.DeviceList, error) {
	if !Supported(pgpu) {
		return nil, nil, nil
	}
	if pgpu.Status.Capabilities == nil || pgpu.Status.Capabilities.MemoryMiB == nil || pgpu.Status.Capabilities.Nvidia == nil || pgpu.Status.Capabilities.Nvidia.MIG == nil {
		return nil, nil, fmt.Errorf("missing mig capabilities for %s", pgpu.Name)
	}
	if session == nil {
		return nil, nil, fmt.Errorf("mig placements unavailable for %s", pgpu.Name)
	}

	pciAddress := invcommon.PCIAddressFor(pgpu)
	profiles := pgpu.Status.Capabilities.Nvidia.MIG.Profiles
	if len(profiles) == 0 {
		return nil, nil, nil
	}

	profileIDs := make([]int32, 0, len(profiles))
	for _, profile := range profiles {
		profileIDs = append(profileIDs, profile.ProfileID)
	}

	placements, err := session.ReadPlacements(pciAddress, profileIDs)
	if err != nil {
		return nil, nil, err
	}

	totalSlices := pgpu.Status.Capabilities.Nvidia.MIG.TotalSlices
	// Placements can reference slice indexes beyond reported total slices.
	if placementSlices := maxMigSlices(placements); placementSlices > totalSlices {
		totalSlices = placementSlices
	}

	totals := totalsFromProfiles(*pgpu.Status.Capabilities.MemoryMiB, totalSlices, profiles)
	counterSet := allocatable.CounterSet{
		Name:     allocatable.CounterSetNameForPCI(pciAddress),
		Counters: buildMigCounters(totals),
	}

	var devices allocatable.DeviceList
	for _, profile := range profiles {
		if profile.MemoryMiB == 0 || profile.SliceCount == 0 {
			continue
		}
		for _, placement := range placements[profile.ProfileID] {
			devices = append(devices, allocatable.NewMIGDevice(
				migDeviceName(pciAddress, profile.ProfileID, placement),
				"",
				invcommon.BaseAttributes(pgpu, gpuv1alpha1.DeviceTypeMIG, profile.Name),
				map[string]allocatable.CapacityValue{
					allocatable.CapMemory: invcommon.MemoryCapacityMiB(int64(profile.MemoryMiB)),
				},
				[]allocatable.CounterConsumption{
					{
						CounterSet: counterSet.Name,
						Counters:   buildMigConsumes(profile, placement),
					},
				},
			))
		}
	}

	return []allocatable.CounterSet{counterSet}, devices, nil
}

// Supported reports whether a PhysicalGPU supports MIG.
func Supported(pgpu gpuv1alpha1.PhysicalGPU) bool {
	if pgpu.Status.Capabilities == nil || pgpu.Status.Capabilities.Nvidia == nil {
		return false
	}
	if pgpu.Status.Capabilities.Nvidia.MIGSupported == nil {
		return false
	}
	return *pgpu.Status.Capabilities.Nvidia.MIGSupported
}

func maxMigSlices(placements map[int32][]invtypes.MigPlacement) int32 {
	var max int32
	for _, profilePlacements := range placements {
		for _, placement := range profilePlacements {
			end := placement.Start + placement.Size
			if end > max {
				max = end
			}
		}
	}
	return max
}
