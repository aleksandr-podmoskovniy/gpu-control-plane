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

// SavePrepareCompletedStep persists the completed checkpoint state.
type SavePrepareCompletedStep struct {
	store ports.PrepareCheckpointStore
}

// NewSavePrepareCompletedStep constructs a step to save a completed checkpoint.
func NewSavePrepareCompletedStep(store ports.PrepareCheckpointStore) SavePrepareCompletedStep {
	return SavePrepareCompletedStep{store: store}
}

func (s SavePrepareCompletedStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if s.store == nil {
		return nil, errors.New("prepare checkpoint store is not configured")
	}
	st.Checkpoint.Claims[st.Request.ClaimUID] = domain.PreparedClaim{
		State:   domain.PrepareStateCompleted,
		Devices: st.DeviceStates,
	}
	if err := s.store.Save(ctx, st.Checkpoint); err != nil {
		return nil, fmt.Errorf("save completed checkpoint: %w", err)
	}
	return nil, nil
}
