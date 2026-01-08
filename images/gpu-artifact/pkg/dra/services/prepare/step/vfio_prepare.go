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

// VfioPrepareStep binds devices to vfio-pci.
type VfioPrepareStep struct {
	vfio ports.VfioManager
}

// NewVfioPrepareStep constructs a VFIO prepare step.
func NewVfioPrepareStep(vfio ports.VfioManager) VfioPrepareStep {
	return VfioPrepareStep{vfio: vfio}
}

func (s VfioPrepareStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if !st.Request.VFIO {
		return nil, nil
	}
	if s.vfio == nil {
		return nil, errors.New("vfio manager is not configured")
	}

	for i, devState := range st.DeviceStates {
		reqDev, ok := st.DeviceMap[devState.Device]
		if !ok {
			return nil, fmt.Errorf("missing prepare device %q", devState.Device)
		}
		deviceType := prepdevice.AttrString(reqDev.Attributes, allocatable.AttrDeviceType)
		if !prepdevice.IsPhysicalDevice(deviceType) {
			continue
		}
		if devState.VFIO != nil {
			continue
		}
		pci := prepdevice.AttrString(reqDev.Attributes, allocatable.AttrPCIAddress)
		if pci == "" {
			return nil, fmt.Errorf("pci address is missing for device %q", devState.Device)
		}
		vfioState, err := s.vfio.Prepare(ctx, domain.VfioPrepareRequest{PCIBusID: pci})
		if err != nil {
			return nil, fmt.Errorf("vfio prepare %q: %w", devState.Device, err)
		}
		st.DeviceStates[i].VFIO = &vfioState
		st.ResourcesChanged = true
	}
	return nil, nil
}
