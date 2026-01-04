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

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func buildMigDevices(pgpu gpuv1alpha1.PhysicalGPU, session MigPlacementSession) ([]allocatable.CounterSet, allocatable.DeviceList, error) {
	if !migSupported(pgpu) {
		return nil, nil, nil
	}
	if pgpu.Status.Capabilities == nil || pgpu.Status.Capabilities.MemoryMiB == nil || pgpu.Status.Capabilities.Nvidia == nil || pgpu.Status.Capabilities.Nvidia.MIG == nil {
		return nil, nil, fmt.Errorf("missing mig capabilities for %s", pgpu.Name)
	}
	if session == nil {
		return nil, nil, fmt.Errorf("mig placements unavailable for %s", pgpu.Name)
	}

	pciAddress := pciAddressFor(pgpu)
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

	counterSet := allocatable.CounterSet{
		Name:     counterSetName(pciAddress),
		Counters: buildMigCounters(*pgpu.Status.Capabilities.MemoryMiB, pgpu.Status.Capabilities.Nvidia.MIG.TotalSlices),
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
				baseAttributes(pgpu, gpuv1alpha1.DeviceTypeMIG, profile.Name),
				map[string]allocatable.CapacityValue{
					allocatable.CapMemory: memoryCapacityMiB(int64(profile.MemoryMiB)),
				},
				[]allocatable.CounterConsumption{
					{
						CounterSet: counterSet.Name,
						Counters:   buildMigConsumes(profile.MemoryMiB, placement),
					},
				},
			))
		}
	}

	return []allocatable.CounterSet{counterSet}, devices, nil
}

func migSupported(pgpu gpuv1alpha1.PhysicalGPU) bool {
	if pgpu.Status.Capabilities == nil || pgpu.Status.Capabilities.Nvidia == nil {
		return false
	}
	if pgpu.Status.Capabilities.Nvidia.MIGSupported == nil {
		return false
	}
	return *pgpu.Status.Capabilities.Nvidia.MIGSupported
}
