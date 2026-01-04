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

package inventory

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
)

// NVMLMigPlacementReader reads MIG placements via NVML.
type NVMLMigPlacementReader struct {
	nvml service.NVML
}

// NewNVMLMigPlacementReader constructs a MIG placement reader.
func NewNVMLMigPlacementReader(nvmlService service.NVML) *NVMLMigPlacementReader {
	return &NVMLMigPlacementReader{nvml: nvmlService}
}

// Open initializes NVML and returns a placement session.
func (r *NVMLMigPlacementReader) Open() (MigPlacementSession, error) {
	if r.nvml == nil {
		return nil, newReadError(ErrNVMLUnavailable, "NVML is not configured")
	}

	ret := r.nvml.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return nil, newReadError(ErrNVMLUnavailable, "NVML init failed: %s", r.nvml.ErrorString(ret))
	}

	return &nvmlMigPlacementSession{nvml: r.nvml}, nil
}

type nvmlMigPlacementSession struct {
	nvml service.NVML
}

func (s *nvmlMigPlacementSession) Close() {
	if s.nvml != nil {
		s.nvml.Shutdown()
	}
}

func (s *nvmlMigPlacementSession) ReadPlacements(pciAddress string, profileIDs []int32) (map[int32][]MigPlacement, error) {
	if pciAddress == "" {
		return nil, newReadError(ErrNVMLQueryFailed, "missing PCI address")
	}
	dev, ret := s.nvml.DeviceByPCI(pciAddress)
	if ret != nvml.SUCCESS {
		return nil, newReadError(ErrNVMLUnavailable, "NVML device lookup failed: %s", s.nvml.ErrorString(ret))
	}

	target := make(map[uint32]struct{}, len(profileIDs))
	for _, id := range profileIDs {
		if id > 0 {
			target[uint32(id)] = struct{}{}
		}
	}

	placements := make(map[int32][]MigPlacement)
	if len(target) == 0 {
		return placements, nil
	}

	for profile := 0; profile < nvml.GPU_INSTANCE_PROFILE_COUNT; profile++ {
		info, ret := dev.GetGpuInstanceProfileInfo(profile)
		switch ret {
		case nvml.SUCCESS:
		case nvml.ERROR_NOT_SUPPORTED, nvml.ERROR_INVALID_ARGUMENT:
			continue
		default:
			return nil, newReadError(ErrNVMLQueryFailed, "NVML profile info failed: %s", s.nvml.ErrorString(ret))
		}

		if _, ok := target[info.Id]; !ok {
			continue
		}

		possible, ret := dev.GetGpuInstancePossiblePlacements(&info)
		switch ret {
		case nvml.SUCCESS:
		case nvml.ERROR_NOT_SUPPORTED, nvml.ERROR_INVALID_ARGUMENT:
			continue
		default:
			return nil, newReadError(ErrNVMLQueryFailed, "NVML placements failed: %s", s.nvml.ErrorString(ret))
		}

		for _, placement := range possible {
			placements[int32(info.Id)] = append(placements[int32(info.Id)], MigPlacement{
				Start: int32(placement.Start),
				Size:  int32(placement.Size),
			})
		}
	}

	for id := range target {
		if _, ok := placements[int32(id)]; !ok {
			return nil, newReadError(ErrNVMLQueryFailed, "NVML placements missing for profile %d", id)
		}
	}

	return placements, nil
}
