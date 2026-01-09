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

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// TimeSlicingUnprepareStep resets time-slicing settings for prepared devices.
type TimeSlicingUnprepareStep struct {
	timeSlicing ports.TimeSlicingManager
}

// NewTimeSlicingUnprepareStep constructs a time-slicing unprepare step.
func NewTimeSlicingUnprepareStep(timeSlicing ports.TimeSlicingManager) TimeSlicingUnprepareStep {
	return TimeSlicingUnprepareStep{timeSlicing: timeSlicing}
}

func (s TimeSlicingUnprepareStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if st.Skip {
		return nil, nil
	}

	timeSliceUUIDs := map[string]struct{}{}
	for _, dev := range st.Claim.Devices {
		if dev.Sharing == nil || dev.Sharing.Strategy != configapi.TimeSlicingStrategy {
			continue
		}
		if dev.Sharing.DeviceUUID == "" {
			continue
		}
		timeSliceUUIDs[dev.Sharing.DeviceUUID] = struct{}{}
	}

	if len(timeSliceUUIDs) == 0 {
		return nil, nil
	}
	if s.timeSlicing == nil {
		return nil, errors.New("time-slicing manager is not configured")
	}

	uuids := make([]string, 0, len(timeSliceUUIDs))
	for uuid := range timeSliceUUIDs {
		uuids = append(uuids, uuid)
	}
	cfg := &configapi.TimeSlicingConfig{Interval: ptr.To(configapi.DefaultTimeSlice)}
	if err := s.timeSlicing.SetTimeSlice(ctx, uuids, cfg); err != nil {
		return nil, err
	}
	return nil, nil
}
