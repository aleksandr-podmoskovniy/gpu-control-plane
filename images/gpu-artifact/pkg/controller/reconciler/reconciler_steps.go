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
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func (r *BaseReconciler[H]) runHandlers(ctx context.Context) (reconcile.Result, error) {
	var result reconcile.Result
	var errs error

handlersLoop:
	for _, h := range r.handlers {
		name := handlerName(h)
		handlerLog, handlerCtx := logger.GetHandlerContext(ctx, name)
		res, err := r.execute(handlerCtx, h)
		switch {
		case err == nil:
		case errors.Is(err, ErrStopHandlerChain):
			handlerLog.Debug("Handler chain execution stopped")
			result = MergeResults(result, res)
			break handlersLoop
		case k8serrors.IsConflict(err):
			handlerLog.Debug("Conflict occurred during handler execution", logger.SlogErr(err))
			result.RequeueAfter = 100 * time.Microsecond
		default:
			handlerLog.Error("Handler failed with an error", logger.SlogErr(err))
			errs = errors.Join(errs, err)
		}

		result = MergeResults(result, res)
	}

	return result, errs
}

func (r *BaseReconciler[H]) updateResource(ctx context.Context, result *reconcile.Result) error {
	err := r.update(ctx)
	switch {
	case err == nil:
		return nil
	case k8serrors.IsConflict(err):
		logger.FromContext(ctx).Debug("Conflict occurred during resource update", logger.SlogErr(err))
		result.RequeueAfter = 100 * time.Microsecond
		return nil
	case strings.Contains(err.Error(), "no new finalizers can be added if the object is being deleted"):
		logger.FromContext(ctx).Warn("Forbidden to add finalizers", logger.SlogErr(err))
		result.RequeueAfter = 1 * time.Second
		return nil
	default:
		logger.FromContext(ctx).Error("Failed to update resource", logger.SlogErr(err))
		return err
	}
}

func (r *BaseReconciler[H]) runFinalizers(ctx context.Context) error {
	for _, h := range r.handlers {
		if finalizer, ok := any(h).(Finalizer); ok {
			if err := finalizer.Finalize(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
