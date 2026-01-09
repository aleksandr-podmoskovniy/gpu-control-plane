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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// MpsUnprepareStep stops MPS control daemons for prepared devices.
type MpsUnprepareStep struct {
	mps ports.MpsManager
}

// NewMpsUnprepareStep constructs an MPS unprepare step.
func NewMpsUnprepareStep(mps ports.MpsManager) MpsUnprepareStep {
	return MpsUnprepareStep{mps: mps}
}

func (s MpsUnprepareStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if st.Skip {
		return nil, nil
	}

	mpsStates := map[string]domain.PreparedMpsState{}
	for _, dev := range st.Claim.Devices {
		if dev.Sharing == nil || dev.Sharing.Strategy != configapi.MpsStrategy || dev.Sharing.MPS == nil {
			continue
		}
		mpsStates[dev.Sharing.MPS.ControlID] = *dev.Sharing.MPS
	}

	if len(mpsStates) == 0 {
		return nil, nil
	}
	if s.mps == nil {
		return nil, errors.New("mps manager is not configured")
	}

	for _, state := range mpsStates {
		if err := s.mps.Stop(ctx, state); err != nil {
			return nil, err
		}
	}
	return nil, nil
}
