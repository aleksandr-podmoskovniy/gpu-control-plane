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

package physicalgpu

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/watcher"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/reconciler"
)

// Handler processes a PhysicalGPU reconciliation step.
type Handler interface {
	Handle(ctx context.Context, s state.PhysicalGPUState) (reconcile.Result, error)
	Name() string
}

// Watcher registers watches for the controller.
type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

// Reconciler reconciles PhysicalGPU resources.
type Reconciler struct {
	client   client.Client
	handlers []Handler
}

// NewReconciler creates a new PhysicalGPU reconciler.
func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

// SetupController registers watches for the PhysicalGPU controller.
func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &gpuv1alpha1.PhysicalGPU{}, &handler.TypedEnqueueRequestForObject[*gpuv1alpha1.PhysicalGPU]{})); err != nil {
		return fmt.Errorf("error setting watch on PhysicalGPU: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewNodeWatcher(mgr.GetCache()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

// Reconcile runs the handler chain on a PhysicalGPU resource.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	physicalGPU := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)
	if err := physicalGPU.Fetch(ctx); err != nil {
		return reconcile.Result{}, err
	}
	if physicalGPU.IsEmpty() {
		return reconcile.Result{}, nil
	}

	s := state.New(r.client, physicalGPU)

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return physicalGPU.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *gpuv1alpha1.PhysicalGPU {
	return &gpuv1alpha1.PhysicalGPU{}
}

func (r *Reconciler) statusGetter(obj *gpuv1alpha1.PhysicalGPU) gpuv1alpha1.PhysicalGPUStatus {
	return obj.Status
}
