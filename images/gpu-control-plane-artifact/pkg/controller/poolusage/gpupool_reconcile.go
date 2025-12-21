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

package poolusage

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	pustate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/poolusage/internal/state"
	ctrlreconciler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

func (r *GPUPoolUsageReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("pool", req.String())
	ctx = logr.NewContext(ctx, log)

	if r.store != nil && !r.store.Current().Enabled {
		log.V(2).Info("module disabled, skipping pool usage reconciliation")
		return reconcile.Result{}, nil
	}

	pool := &v1alpha1.GPUPool{}
	if err := r.client.Get(ctx, req.NamespacedName, pool); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	s := pustate.NewGPUPool(r.client, pool)

	rec := ctrlreconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler Handler) (reconcile.Result, error) {
		return handler.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		if !s.UsedSet() {
			return nil
		}
		return r.patchStatus(ctx, req.NamespacedName, s.Used())
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return reconcile.Result{}, err
	}

	return res, nil
}
