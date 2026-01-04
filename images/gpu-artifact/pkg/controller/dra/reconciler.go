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

package dra

import (
	"context"
	"fmt"
	"reflect"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/watcher"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/reconciler"
)

// Handler processes a ResourceClaim reconciliation step.
type Handler interface {
	Handle(ctx context.Context, s *state.DRAState) (reconcile.Result, error)
	Name() string
}

// Watcher registers watches for the controller.
type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

// Reconciler reconciles ResourceClaim objects.
type Reconciler struct {
	client   client.Client
	handlers []Handler
}

// NewReconciler creates a new DRA reconciler.
func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

// SetupController registers watches for the DRA controller.
func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &resourcev1.ResourceClaim{}, &handler.TypedEnqueueRequestForObject[*resourcev1.ResourceClaim]{})); err != nil {
		return fmt.Errorf("error setting watch on ResourceClaim: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewDeviceClassWatcher(mgr.GetCache()),
		watcher.NewResourceSliceWatcher(mgr.GetCache()),
	} {
		if err := w.Watch(mgr, ctr); err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

// Reconcile runs the handler chain on a ResourceClaim.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	claim := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)
	if err := claim.Fetch(ctx); err != nil {
		return reconcile.Result{}, err
	}
	if claim.IsEmpty() {
		return reconcile.Result{}, nil
	}

	st := state.New(r.client, claim)

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, st)
	})
	rec.SetResourceUpdater(func(_ context.Context) error {
		return nil
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{}
}

func (r *Reconciler) statusGetter(obj *resourcev1.ResourceClaim) resourcev1.ResourceClaimStatus {
	return obj.Status
}
