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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// VfioUnprepareStep restores original GPU drivers.
type VfioUnprepareStep struct {
	vfio ports.VfioManager
}

// NewVfioUnprepareStep constructs a VFIO unprepare step.
func NewVfioUnprepareStep(vfio ports.VfioManager) VfioUnprepareStep {
	return VfioUnprepareStep{vfio: vfio}
}

func (s VfioUnprepareStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if st.Skip || s.vfio == nil {
		return nil, nil
	}
	for _, dev := range st.Claim.Devices {
		if dev.VFIO == nil {
			continue
		}
		if err := s.vfio.Unprepare(ctx, *dev.VFIO); err != nil {
			return nil, fmt.Errorf("vfio unprepare %q: %w", dev.Device, err)
		}
		st.ResourcesChanged = true
	}
	return nil, nil
}
