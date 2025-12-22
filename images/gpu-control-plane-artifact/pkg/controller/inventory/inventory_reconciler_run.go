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

package inventory

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	ctrlreconciler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.Name)
	ctx = logr.NewContext(ctx, logger)

	node := &corev1.Node{}
	node, err := commonobject.FetchObject(ctx, req.NamespacedName, r.client, node)
	if err != nil {
		return ctrl.Result{}, err
	}
	if node == nil {
		// Rely on ownerReferences GC; avoid aggressive cleanup that may fire on transient cache misses.
		logger.V(1).Info("node removed, skipping reconciliation")
		r.cleanupSvc().ClearMetrics(req.Name)
		return ctrl.Result{}, nil
	}

	managedPolicy, approvalPolicy := r.currentPolicies()

	nodeFeature, err := invstate.FindNodeFeature(ctx, r.client, node.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	state := invstate.NewInventoryState(node, nodeFeature, managedPolicy, approvalPolicy)

	rec := ctrlreconciler.NewBaseReconciler[Handler](r.handlerChain())
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, state)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error { return nil })

	return rec.Reconcile(ctx)
}
