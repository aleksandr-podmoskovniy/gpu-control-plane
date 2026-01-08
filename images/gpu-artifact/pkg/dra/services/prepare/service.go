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

package prepare

import (
	"context"
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// Service prepares and unprepares devices using node-local side effects.
type Service struct {
	cdi         ports.CDIWriter
	mig         ports.MigManager
	vfio        ports.VfioManager
	timeSlicing ports.TimeSlicingManager
	mps         ports.MpsManager
	locker      ports.PrepareLocker
	checkpoints ports.PrepareCheckpointStore
	checker     ports.GPUProcessChecker
	notifier    ports.ResourcesChangeNotifier
	prepare     steptaker.StepTakers[*state.PrepareState]
	unprepare   steptaker.StepTakers[*state.UnprepareState]
}

// Options configure the prepare service.
type Options struct {
	CDI               ports.CDIWriter
	MIG               ports.MigManager
	VFIO              ports.VfioManager
	TimeSlicing       ports.TimeSlicingManager
	Mps               ports.MpsManager
	Locker            ports.PrepareLocker
	Checkpoints       ports.PrepareCheckpointStore
	GPUChecker        ports.GPUProcessChecker
	ResourcesNotifier ports.ResourcesChangeNotifier
}

// NewService creates a prepare Service.
func NewService(opts Options) (*Service, error) {
	if opts.CDI == nil {
		return nil, errors.New("CDI writer is required")
	}
	if opts.Locker == nil {
		return nil, errors.New("prepare locker is required")
	}
	if opts.Checkpoints == nil {
		return nil, errors.New("checkpoint store is required")
	}
	return &Service{
		cdi:         opts.CDI,
		mig:         opts.MIG,
		vfio:        opts.VFIO,
		timeSlicing: opts.TimeSlicing,
		mps:         opts.Mps,
		locker:      opts.Locker,
		checkpoints: opts.Checkpoints,
		checker:     opts.GPUChecker,
		notifier:    opts.ResourcesNotifier,
		prepare:     newPrepareSteps(opts),
		unprepare:   newUnprepareSteps(opts),
	}, nil
}

// Prepare writes CDI specs and returns prepared devices for the claim.
func (s *Service) Prepare(ctx context.Context, req domain.PrepareRequest) (domain.PrepareResult, error) {
	if s == nil || s.cdi == nil {
		return domain.PrepareResult{}, errors.New("CDI writer is not configured")
	}
	st := state.NewPrepareState(req)
	defer func() {
		if st.Unlock != nil {
			_ = st.Unlock()
		}
	}()
	if _, err := s.prepare.Run(ctx, st); err != nil {
		return domain.PrepareResult{}, err
	}
	if st.Result.ClaimUID == "" {
		return domain.PrepareResult{}, errors.New("prepare completed without result")
	}
	return st.Result, nil
}

// Unprepare removes CDI specs for the claim.
func (s *Service) Unprepare(ctx context.Context, claimUID string) error {
	if s == nil || s.cdi == nil {
		return errors.New("prepare service is not configured")
	}
	st := state.NewUnprepareState(claimUID)
	defer func() {
		if st.Unlock != nil {
			_ = st.Unlock()
		}
	}()
	_, err := s.unprepare.Run(ctx, st)
	return err
}
