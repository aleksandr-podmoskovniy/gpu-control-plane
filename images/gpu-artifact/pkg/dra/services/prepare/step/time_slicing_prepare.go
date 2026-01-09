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

// TimeSlicingPrepareStep applies time-slicing settings to physical GPUs.
type TimeSlicingPrepareStep struct {
	timeSlicing ports.TimeSlicingManager
}

// NewTimeSlicingPrepareStep constructs a time-slicing prepare step.
func NewTimeSlicingPrepareStep(timeSlicing ports.TimeSlicingManager) TimeSlicingPrepareStep {
	return TimeSlicingPrepareStep{timeSlicing: timeSlicing}
}

func (s TimeSlicingPrepareStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
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

	groups := map[string]*timeSlicingGroup{}
	for _, dev := range st.MutableRequest.Devices {
		cfg, ok := dev.Config.(*configapi.GpuConfig)
		if !ok || cfg.Sharing == nil || !cfg.Sharing.IsTimeSlicing() {
			continue
		}
		deviceType := prepdevice.AttrString(dev.Attributes, allocatable.AttrDeviceType)
		if !prepdevice.IsPhysicalDevice(deviceType) {
			return nil, fmt.Errorf("time-slicing requires physical device %q", dev.Device)
		}
		stateIdx, ok := deviceStateIndex[dev.Device]
		if !ok {
			return nil, fmt.Errorf("missing device state for %q", dev.Device)
		}
		reqIdx, ok := requestIndex[dev.Device]
		if !ok {
			return nil, fmt.Errorf("missing mutable request for %q", dev.Device)
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
		group := groups[key]
		if group == nil {
			group = &timeSlicingGroup{config: tsc}
			groups[key] = group
		}
		group.add(reqIdx, stateIdx, uuid)
	}

	if len(groups) == 0 {
		return nil, nil
	}
	if s.timeSlicing == nil {
		return nil, errors.New("time-slicing manager is not configured")
	}

	for _, group := range groups {
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

	return nil, nil
}
