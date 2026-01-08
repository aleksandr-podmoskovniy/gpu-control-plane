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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/checkpoint"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// LoadPrepareCheckpointStep loads prepare checkpoints.
type LoadPrepareCheckpointStep struct {
	store ports.PrepareCheckpointStore
}

// NewLoadPrepareCheckpointStep constructs a checkpoint load step.
func NewLoadPrepareCheckpointStep(store ports.PrepareCheckpointStore) LoadPrepareCheckpointStep {
	return LoadPrepareCheckpointStep{store: store}
}

func (s LoadPrepareCheckpointStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if s.store == nil {
		return nil, errors.New("prepare checkpoint store is not configured")
	}
	cp, err := checkpoint.Load(ctx, s.store)
	if err != nil {
		return nil, err
	}
	st.Checkpoint = cp
	if claim, ok := cp.Claims[st.Request.ClaimUID]; ok {
		st.Claim = claim
	}
	return nil, nil
}

// LoadUnprepareCheckpointStep loads checkpoints for unprepare.
type LoadUnprepareCheckpointStep struct {
	store ports.PrepareCheckpointStore
}

// NewLoadUnprepareCheckpointStep constructs a checkpoint load step for unprepare.
func NewLoadUnprepareCheckpointStep(store ports.PrepareCheckpointStore) LoadUnprepareCheckpointStep {
	return LoadUnprepareCheckpointStep{store: store}
}

func (s LoadUnprepareCheckpointStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if s.store == nil {
		return nil, errors.New("prepare checkpoint store is not configured")
	}
	cp, err := checkpoint.Load(ctx, s.store)
	if err != nil {
		return nil, err
	}
	st.Checkpoint = cp
	if claim, ok := cp.Claims[st.ClaimUID]; ok {
		st.Claim = claim
	}
	return nil, nil
}
