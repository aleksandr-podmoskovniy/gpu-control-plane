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

package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/deckhouse/deckhouse/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

// NewSteps builds a deterministic handler pipeline.
func NewSteps(log *log.Logger, handlers ...Handler) steptaker.StepTakers[state.State] {
	steps := make([]steptaker.StepTaker[state.State], 0, len(handlers)+1)
	for _, h := range handlers {
		steps = append(steps, step{
			handler: h,
			log:     log,
		})
	}
	steps = append(steps, finalStep{})
	return steptaker.NewStepTakers(steps...)
}

type step struct {
	handler Handler
	log     *log.Logger
}

func (s step) Take(ctx context.Context, st state.State) (*reconcile.Result, error) {
	if s.handler == nil {
		return nil, nil
	}
	handlerLog := s.log.With(logger.SlogHandler(s.handler.Name()), logger.SlogStep("run"))
	if err := s.handler.Handle(ctx, st); err != nil {
		if errors.Is(err, ErrStopHandlerChain) {
			handlerLog.Info("handler chain stopped", logger.SlogErr(err))
			res := reconcile.Result{}
			return &res, nil
		}
		handlerLog.Error("handler failed", logger.SlogErr(err))
		return nil, fmt.Errorf("%s: %w", s.handler.Name(), err)
	}
	handlerLog.Debug("handler completed")
	return nil, nil
}

type finalStep struct{}

func (finalStep) Take(_ context.Context, _ state.State) (*reconcile.Result, error) {
	res := reconcile.Result{}
	return &res, nil
}
