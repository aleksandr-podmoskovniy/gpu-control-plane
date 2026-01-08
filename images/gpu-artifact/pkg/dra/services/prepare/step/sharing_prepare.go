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

package step

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	prepdevice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/internal/device"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// SharingPrepareStep applies sharing settings like time-slicing and MPS.
type SharingPrepareStep struct {
	timeSlicing ports.TimeSlicingManager
	mps         ports.MpsManager
}

// NewSharingPrepareStep constructs a sharing prepare step.
func NewSharingPrepareStep(timeSlicing ports.TimeSlicingManager, mps ports.MpsManager) SharingPrepareStep {
	return SharingPrepareStep{timeSlicing: timeSlicing, mps: mps}
}

func (s SharingPrepareStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}

	deviceStateIndex := map[string]int{}
	for i, dev := range st.DeviceStates {
		deviceStateIndex[dev.Device] = i
	}
	requestIndex := map[string]int{}
	for i, dev := range st.MutableRequest.Devices {
		requestIndex[dev.Device] = i
	}

	tsGroups := map[string]*timeSlicingGroup{}
	mpsGroups := map[string]*mpsGroup{}

	for _, dev := range st.MutableRequest.Devices {
		if dev.Config == nil {
			continue
		}
		deviceType := prepdevice.AttrString(dev.Attributes, allocatable.AttrDeviceType)
		stateIdx, ok := deviceStateIndex[dev.Device]
		if !ok {
			return nil, fmt.Errorf("missing device state for %q", dev.Device)
		}
		reqIdx, ok := requestIndex[dev.Device]
		if !ok {
			return nil, fmt.Errorf("missing mutable request for %q", dev.Device)
		}

		switch cfg := dev.Config.(type) {
		case *configapi.GpuConfig:
			if cfg.Sharing == nil {
				continue
			}
			if cfg.Sharing.IsTimeSlicing() {
				if !prepdevice.IsPhysicalDevice(deviceType) {
					return nil, fmt.Errorf("time-slicing requires physical device %q", dev.Device)
				}
				tsc, err := cfg.Sharing.GetTimeSlicingConfig()
				if err != nil {
					return nil, fmt.Errorf("get time-slicing config: %w", err)
				}
				if tsc == nil || tsc.Interval == nil {
					return nil, fmt.Errorf("time-slicing config missing interval for %q", dev.Device)
				}
				uuid := prepdevice.AttrString(dev.Attributes, allocatable.AttrGPUUUID)
				if uuid == "" {
					return nil, fmt.Errorf("gpu uuid is missing for %q", dev.Device)
				}
				key := string(*tsc.Interval)
				group := tsGroups[key]
				if group == nil {
					group = &timeSlicingGroup{config: tsc}
					tsGroups[key] = group
				}
				group.add(reqIdx, stateIdx, uuid)
			}
			if cfg.Sharing.IsMps() {
				mpsCfg, err := cfg.Sharing.GetMpsConfig()
				if err != nil {
					return nil, fmt.Errorf("get mps config: %w", err)
				}
				uuid := prepdevice.AttrString(dev.Attributes, allocatable.AttrGPUUUID)
				if uuid == "" {
					return nil, fmt.Errorf("gpu uuid is missing for %q", dev.Device)
				}
				group := ensureMpsGroup(mpsGroups, deviceType, mpsCfg)
				if err := mergeExistingMpsState(group, st.DeviceStates[stateIdx].Sharing); err != nil {
					return nil, err
				}
				group.add(reqIdx, stateIdx, uuid)
			}
		case *configapi.MigDeviceConfig:
			if cfg.Sharing == nil {
				continue
			}
			if cfg.Sharing.IsMps() {
				mpsCfg, err := cfg.Sharing.GetMpsConfig()
				if err != nil {
					return nil, fmt.Errorf("get mps config: %w", err)
				}
				uuid := prepdevice.AttrString(dev.Attributes, allocatable.AttrMigUUID)
				if uuid == "" {
					return nil, fmt.Errorf("mig uuid is missing for %q", dev.Device)
				}
				group := ensureMpsGroup(mpsGroups, deviceType, mpsCfg)
				if err := mergeExistingMpsState(group, st.DeviceStates[stateIdx].Sharing); err != nil {
					return nil, err
				}
				group.add(reqIdx, stateIdx, uuid)
			}
		case *configapi.VfioDeviceConfig:
			continue
		default:
			return nil, fmt.Errorf("unsupported config type %T", dev.Config)
		}
	}

	if len(tsGroups) > 0 {
		if s.timeSlicing == nil {
			return nil, errors.New("time-slicing manager is not configured")
		}
		for _, group := range tsGroups {
			uuids := uniqueStrings(group.deviceUUIDs)
			if err := s.timeSlicing.SetTimeSlice(ctx, uuids, group.config); err != nil {
				return nil, err
			}
			for _, idx := range group.stateIndexes {
				applySharingState(&st.DeviceStates[idx], domain.PreparedSharing{
					Strategy:   configapi.TimeSlicingStrategy,
					DeviceUUID: group.deviceUUIDs[group.deviceIndex[idx]],
				})
			}
		}
	}

	if len(mpsGroups) > 0 {
		if s.mps == nil {
			return nil, errors.New("mps manager is not configured")
		}
		for _, group := range mpsGroups {
			controlID := ""
			if group.existing != nil {
				controlID = group.existing.ControlID
			}
			uuids := uniqueStrings(group.deviceUUIDs)
			if controlID == "" {
				controlID = buildMpsControlID(st.Request.ClaimUID, group.key, append([]string(nil), uuids...))
			}
			state, err := s.mps.Start(ctx, domain.MpsPrepareRequest{
				ControlID:   controlID,
				DeviceUUIDs: uuids,
				Config:      group.config,
			})
			if err != nil {
				return nil, err
			}
			mpsState := &state
			for _, idx := range group.reqIndexes {
				applyMpsAttributes(&st.MutableRequest.Devices[idx], *mpsState)
			}
			for _, idx := range group.stateIndexes {
				applySharingState(&st.DeviceStates[idx], domain.PreparedSharing{
					Strategy:   configapi.MpsStrategy,
					DeviceUUID: group.deviceUUIDs[group.deviceIndex[idx]],
					MPS:        mpsState,
				})
			}
		}
	}

	return nil, nil
}
