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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	prepdevice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/internal/device"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// MigPrepareStep creates missing MIG instances.
type MigPrepareStep struct {
	mig ports.MigManager
}

// NewMigPrepareStep constructs a MIG prepare step.
func NewMigPrepareStep(mig ports.MigManager) MigPrepareStep {
	return MigPrepareStep{mig: mig}
}

func (s MigPrepareStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if s.mig == nil {
		return nil, nil
	}

	for i, devState := range st.DeviceStates {
		reqDev, ok := st.DeviceMap[devState.Device]
		if !ok {
			return nil, fmt.Errorf("missing prepare device %q", devState.Device)
		}
		deviceType := prepdevice.AttrString(reqDev.Attributes, allocatable.AttrDeviceType)
		if !prepdevice.IsMigDevice(deviceType) {
			continue
		}
		if devState.MIG != nil {
			applyMigUUID(st, devState.Device, devState.MIG.DeviceUUID)
			continue
		}
		migReq, err := prepdevice.BuildMigPrepareRequest(reqDev)
		if err != nil {
			return nil, err
		}
		migState, err := s.mig.Prepare(ctx, migReq)
		if err != nil {
			return nil, fmt.Errorf("mig prepare %q: %w", devState.Device, err)
		}
		st.DeviceStates[i].MIG = &migState
		st.ResourcesChanged = true
		applyMigUUID(st, devState.Device, migState.DeviceUUID)
	}
	return nil, nil
}

func applyMigUUID(st *state.PrepareState, deviceName, uuid string) {
	if st == nil || uuid == "" {
		return
	}
	for i := range st.MutableRequest.Devices {
		if st.MutableRequest.Devices[i].Device != deviceName {
			continue
		}
		attrs := st.MutableRequest.Devices[i].Attributes
		if attrs == nil {
			attrs = map[string]allocatable.AttributeValue{}
		}
		attrs[allocatable.AttrMigUUID] = allocatable.AttributeValue{String: &uuid}
		st.MutableRequest.Devices[i].Attributes = attrs
		return
	}
}
