// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconciler

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ErrStopHandlerChain is a sentinel error allowing handlers to stop further execution.
var ErrStopHandlerChain = errors.New("stop handler chain execution")

const conflictRequeueAfter = 100 * time.Microsecond

type (
	// Named marks handlers that expose human readable name for logging.
	Named interface {
		Name() string
	}

	// Finalizer marks handlers that require follow-up finalization after the main loop.
	Finalizer interface {
		Finalize(ctx context.Context) error
	}

	// ResourceUpdater persists the final resource state.
	ResourceUpdater func(ctx context.Context) error

	// HandlerExecutor runs a single handler and returns its reconcile.Result.
	HandlerExecutor[H any] func(ctx context.Context, h H) (reconcile.Result, error)
)

// BaseReconciler orchestrates handler execution, resource updates and finalizers.
type BaseReconciler[H any] struct {
	handlers []H
	update   ResourceUpdater
	execute  HandlerExecutor[H]
}

// NewBaseReconciler constructs a BaseReconciler for the provided handlers.
func NewBaseReconciler[H any](handlers []H) *BaseReconciler[H] {
	return &BaseReconciler[H]{handlers: handlers}
}

// SetResourceUpdater configures the resource update callback.
func (b *BaseReconciler[H]) SetResourceUpdater(update ResourceUpdater) {
	b.update = update
}

// SetHandlerExecutor configures how individual handlers are invoked.
func (b *BaseReconciler[H]) SetHandlerExecutor(execute HandlerExecutor[H]) {
	b.execute = execute
}

// Reconcile executes handlers sequentially, applies updates and finalizers.
func (b *BaseReconciler[H]) Reconcile(ctx context.Context) (reconcile.Result, error) {
	if b.update == nil {
		return reconcile.Result{}, errors.New("resource updater is not configured")
	}
	if b.execute == nil {
		return reconcile.Result{}, errors.New("handler executor is not configured")
	}

	log := logr.FromContextOrDiscard(ctx)
	log.V(2).Info("start reconciliation")

	var (
		result reconcile.Result
		errs   error
	)

	for _, handler := range b.handlers {
		handlerName := reflect.TypeOf(handler).String()
		if named, ok := any(handler).(Named); ok {
			handlerName = named.Name()
		}

		handlerLog := log.WithValues("handler", handlerName)
		handlerCtx := logr.NewContext(ctx, handlerLog)

		res, err := b.execute(handlerCtx, handler)

		switch {
		case err == nil:
			// noop
		case errors.Is(err, ErrStopHandlerChain):
			handlerLog.V(1).Info("handler requested to stop chain")
			result = MergeResults(result, res)
			goto finalize // skip remaining handlers
		case k8serrors.IsConflict(err):
			handlerLog.V(1).Info("conflict occurred during handler execution", "err", err)
			res = MergeResults(res, reconcile.Result{RequeueAfter: conflictRequeueAfter})
		default:
			handlerLog.Error(err, "handler failed")
			errs = errors.Join(errs, err)
		}

		result = MergeResults(result, res)

		if errs != nil && !k8serrors.IsConflict(errs) {
			break
		}
	}

finalize:
	if err := b.update(ctx); err != nil {
		switch {
		case k8serrors.IsConflict(err):
			log.V(1).Info("conflict occurred during resource update", "err", err)
			result = MergeResults(result, reconcile.Result{RequeueAfter: conflictRequeueAfter})
		default:
			log.Error(err, "failed to persist resource changes")
			errs = errors.Join(errs, err)
		}
	}

	for _, handler := range b.handlers {
		finalizer, ok := any(handler).(Finalizer)
		if !ok {
			continue
		}
		if err := finalizer.Finalize(ctx); err != nil {
			log.Error(err, "failed to finalize handler", "handler", reflect.TypeOf(handler).String())
			errs = errors.Join(errs, err)
		}
	}

	if errs != nil {
		log.Error(errs, "handler chain finished with errors")
		return reconcile.Result{}, errs
	}

	log.V(2).Info("reconciliation completed",
		"requeue", result.Requeue,
		"requeueAfter", result.RequeueAfter)

	return result, nil
}
