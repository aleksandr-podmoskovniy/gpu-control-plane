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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// Handler performs a single action against the gpu-handler state.
type Handler interface {
	Name() string
	Handle(ctx context.Context, st state.State) error
}

// ErrStopHandlerChain stops handler execution immediately.
var ErrStopHandlerChain = errors.New("stop handler chain")

// Chain executes handlers in order.
type Chain struct {
	handlers []Handler
}

// NewChain builds a handler chain.
func NewChain(handlers ...Handler) Chain {
	return Chain{handlers: handlers}
}

// Run executes handlers and aggregates their errors.
func (c Chain) Run(ctx context.Context, st state.State, log *log.Logger) error {
	var errs []error
	for _, h := range c.handlers {
		handlerLog := log.With(logger.SlogHandler(h.Name()), logger.SlogStep("run"))
		if err := h.Handle(ctx, st); err != nil {
			wrapped := fmt.Errorf("%s: %w", h.Name(), err)
			if errors.Is(err, ErrStopHandlerChain) {
				handlerLog.Info("handler chain stopped", logger.SlogErr(err))
				return wrapped
			}
			handlerLog.Error("handler failed", logger.SlogErr(err))
			errs = append(errs, wrapped)
			continue
		}
		handlerLog.Debug("handler completed")
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
