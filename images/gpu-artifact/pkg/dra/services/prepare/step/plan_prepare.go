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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	prepdevice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/internal/device"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// PlanPrepareStep builds device state and mutable request.
type PlanPrepareStep struct {
	mig  ports.MigManager
	vfio ports.VfioManager
}

// NewPlanPrepareStep constructs a plan step.
func NewPlanPrepareStep(mig ports.MigManager, vfio ports.VfioManager) PlanPrepareStep {
	return PlanPrepareStep{mig: mig, vfio: vfio}
}

func (s PlanPrepareStep) Take(_ context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}

	st.DeviceMap = make(map[string]domain.PrepareDevice, len(st.Request.Devices))
	known := map[string]domain.PreparedDeviceState{}
	for _, dev := range st.Claim.Devices {
		known[dev.Device] = dev
	}

	prepared := make([]domain.PreparedDeviceState, 0, len(st.Request.Devices))
	mutable := st.Request
	mutable.Devices = make([]domain.PrepareDevice, 0, len(st.Request.Devices))

	for _, dev := range st.Request.Devices {
		st.DeviceMap[dev.Device] = dev

		stateItem, ok := known[dev.Device]
		if !ok {
			stateItem = domain.PreparedDeviceState{
				Request: dev.Request,
				Pool:    dev.Pool,
				Device:  dev.Device,
			}
		}
		stateItem.Request = dev.Request
		stateItem.Pool = dev.Pool

		deviceType := prepdevice.AttrString(dev.Attributes, allocatable.AttrDeviceType)
		if st.Request.VFIO {
			if !prepdevice.IsPhysicalDevice(deviceType) {
				return nil, fmt.Errorf("vfio requested for non-physical device %q", dev.Device)
			}
			if dev.ShareID != "" || len(dev.ConsumedCapacity) > 0 {
				return nil, fmt.Errorf("vfio requires exclusive allocation for device %q", dev.Device)
			}
			if s.vfio == nil {
				return nil, errors.New("vfio manager is not configured")
			}
		}

		if prepdevice.IsMigDevice(deviceType) && s.mig == nil {
			return nil, errors.New("mig manager is not configured")
		}

		mutableDev := dev
		mutableDev.Attributes = prepdevice.CloneAttributes(dev.Attributes)
		if stateItem.MIG != nil {
			mutableDev.Attributes[allocatable.AttrMigUUID] = allocatable.AttributeValue{String: &stateItem.MIG.DeviceUUID}
		}
		mutable.Devices = append(mutable.Devices, mutableDev)
		prepared = append(prepared, stateItem)
	}

	st.DeviceStates = prepared
	st.MutableRequest = mutable
	return nil, nil
}
