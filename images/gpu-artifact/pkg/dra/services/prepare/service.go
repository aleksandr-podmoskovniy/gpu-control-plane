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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
)

// Service runs a prepare/unprepare pipeline.
type Service struct {
	steps steptaker.StepTakers[*domain.PrepareRequest]
}

// NewService creates a new prepare service.
func NewService(store ports.CheckpointStore, cdi ports.CDIWriter, hook ports.HookWriter) *Service {
	steps := steptaker.NewStepTakers[*domain.PrepareRequest](
		checkpointStep{store: store},
		cdiStep{writer: cdi},
		hookStep{writer: hook},
		finishStep{},
	)
	return &Service{steps: steps}
}

// RunOnce executes a single prepare pipeline.
func (s *Service) RunOnce(ctx context.Context, req domain.PrepareRequest) error {
	_, err := s.steps.Run(ctx, &req)
	return err
}

type checkpointStep struct {
	store ports.CheckpointStore
}

func (s checkpointStep) Take(ctx context.Context, req *domain.PrepareRequest) (*reconcile.Result, error) {
	if req == nil {
		return &reconcile.Result{}, nil
	}
	return nil, s.store.Save(ctx, *req)
}

type cdiStep struct {
	writer ports.CDIWriter
}

func (s cdiStep) Take(ctx context.Context, req *domain.PrepareRequest) (*reconcile.Result, error) {
	if req == nil {
		return &reconcile.Result{}, nil
	}
	return nil, s.writer.Write(ctx, *req)
}

type hookStep struct {
	writer ports.HookWriter
}

func (s hookStep) Take(ctx context.Context, req *domain.PrepareRequest) (*reconcile.Result, error) {
	if req == nil {
		return &reconcile.Result{}, nil
	}
	return nil, s.writer.Write(ctx, *req)
}

type finishStep struct{}

func (finishStep) Take(_ context.Context, _ *domain.PrepareRequest) (*reconcile.Result, error) {
	return &reconcile.Result{}, nil
}

// DefaultRequest returns an empty prepare request.
func DefaultRequest() domain.PrepareRequest {
	return domain.PrepareRequest{}
}
