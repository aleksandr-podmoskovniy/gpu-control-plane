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

// MigUnprepareStep removes MIG instances.
type MigUnprepareStep struct {
	mig ports.MigManager
}

// NewMigUnprepareStep constructs a MIG unprepare step.
func NewMigUnprepareStep(mig ports.MigManager) MigUnprepareStep {
	return MigUnprepareStep{mig: mig}
}

func (s MigUnprepareStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if st.Skip || s.mig == nil {
		return nil, nil
	}
	for _, dev := range st.Claim.Devices {
		if dev.MIG == nil {
			continue
		}
		if err := s.mig.Unprepare(ctx, *dev.MIG); err != nil {
			return nil, fmt.Errorf("mig unprepare %q: %w", dev.Device, err)
		}
		st.ResourcesChanged = true
	}
	return nil, nil
}
