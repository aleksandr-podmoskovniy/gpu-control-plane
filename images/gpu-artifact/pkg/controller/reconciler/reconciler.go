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

package reconciler

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// ErrStopHandlerChain stops handler execution without error.
var ErrStopHandlerChain = errors.New("stop handler chain execution")

// Named provides a stable handler name.
type Named interface {
	Name() string
}

// Finalizer runs after the resource update.
type Finalizer interface {
	Finalize(ctx context.Context) error
}

// ResourceUpdater updates the reconciled resource.
type ResourceUpdater func(ctx context.Context) error

// HandlerExecutor executes a handler and returns its result.
type HandlerExecutor[H any] func(ctx context.Context, h H) (reconcile.Result, error)

// BaseReconciler runs a chain of handlers and updates the resource.
type BaseReconciler[H any] struct {
	handlers []H
	update   ResourceUpdater
	execute  HandlerExecutor[H]
}

// NewBaseReconciler creates a BaseReconciler.
func NewBaseReconciler[H any](handlers []H) *BaseReconciler[H] {
	return &BaseReconciler[H]{
		handlers: handlers,
	}
}

// SetResourceUpdater sets the resource update function.
func (r *BaseReconciler[H]) SetResourceUpdater(update ResourceUpdater) {
	r.update = update
}

// SetHandlerExecutor sets the handler execution function.
func (r *BaseReconciler[H]) SetHandlerExecutor(execute HandlerExecutor[H]) {
	r.execute = execute
}

// Reconcile runs handlers, updates resource, and returns a combined result.
func (r *BaseReconciler[H]) Reconcile(ctx context.Context) (reconcile.Result, error) {
	if r.update == nil {
		return reconcile.Result{}, errors.New("update func is omitted: cannot reconcile")
	}
	if r.execute == nil {
		return reconcile.Result{}, errors.New("execute func is omitted: cannot reconcile")
	}

	logger.FromContext(ctx).Debug("Start reconciliation")

	result, errs := r.runHandlers(ctx)
	if err := r.updateResource(ctx, &result); err != nil {
		errs = errors.Join(errs, err)
	}
	if errs != nil {
		logger.FromContext(ctx).Error("Error occurred during reconciliation", logger.SlogErr(errs))
		return reconcile.Result{}, errs
	}
	if err := r.runFinalizers(ctx); err != nil {
		logger.FromContext(ctx).Error("Failed to finalize resource", logger.SlogErr(err))
		return reconcile.Result{}, err
	}

	logger.FromContext(ctx).Debug("Reconciliation was successfully completed", "requeue", result.Requeue, "after", result.RequeueAfter)
	return result, nil
}
