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
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// WriteCDIStep writes CDI specs for prepared devices.
type WriteCDIStep struct {
	cdi ports.CDIWriter
}

// NewWriteCDIStep constructs a CDI write step.
func NewWriteCDIStep(cdi ports.CDIWriter) WriteCDIStep {
	return WriteCDIStep{cdi: cdi}
}

func (s WriteCDIStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if s.cdi == nil {
		return nil, errors.New("cdi writer is not configured")
	}

	deviceIDs, err := s.cdi.Write(ctx, st.MutableRequest)
	if err != nil {
		return nil, err
	}

	resultDevices := make([]domain.PreparedDevice, 0, len(st.DeviceStates))
	for i, devState := range st.DeviceStates {
		ids, ok := deviceIDs[devState.Device]
		if !ok {
			return nil, fmt.Errorf("missing CDI ids for device %q", devState.Device)
		}
		st.DeviceStates[i].CDIDeviceIDs = ids
		resultDevices = append(resultDevices, domain.PreparedDevice{
			Request:      devState.Request,
			Pool:         devState.Pool,
			Device:       devState.Device,
			CDIDeviceIDs: ids,
		})
	}

	st.Result = domain.PrepareResult{
		ClaimUID: st.Request.ClaimUID,
		Devices:  resultDevices,
	}
	return nil, nil
}
