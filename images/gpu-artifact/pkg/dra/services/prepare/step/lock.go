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

// LockPrepareStep serializes prepare operations.
type LockPrepareStep struct {
	locker ports.PrepareLocker
}

// NewLockPrepareStep constructs a lock step for prepare.
func NewLockPrepareStep(locker ports.PrepareLocker) LockPrepareStep {
	return LockPrepareStep{locker: locker}
}

func (s LockPrepareStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if s.locker == nil {
		return nil, errors.New("prepare locker is not configured")
	}
	unlock, err := s.locker.Lock(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire prepare lock: %w", err)
	}
	st.Unlock = unlock
	return nil, nil
}

// LockUnprepareStep serializes unprepare operations.
type LockUnprepareStep struct {
	locker ports.PrepareLocker
}

// NewLockUnprepareStep constructs a lock step for unprepare.
func NewLockUnprepareStep(locker ports.PrepareLocker) LockUnprepareStep {
	return LockUnprepareStep{locker: locker}
}

func (s LockUnprepareStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if s.locker == nil {
		return nil, errors.New("prepare locker is not configured")
	}
	unlock, err := s.locker.Lock(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire prepare lock: %w", err)
	}
	st.Unlock = unlock
	return nil, nil
}
