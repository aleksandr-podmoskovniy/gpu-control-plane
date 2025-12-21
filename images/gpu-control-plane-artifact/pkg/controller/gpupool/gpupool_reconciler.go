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

package gpupool

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	gpstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/gpupool/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/gpupool/internal/watcher"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	ctrlreconciler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/watchers"
)

type Handler interface {
	Name() string
	Handle(ctx context.Context, s gpstate.PoolState) (reconcile.Result, error)
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Reconciler struct {
	client   client.Client
	log      logr.Logger
	cfg      config.ControllerConfig
	store    *moduleconfig.ModuleConfigStore
	handlers []Handler
}

func NewReconciler(log logr.Logger, cfg config.ControllerConfig, store *moduleconfig.ModuleConfigStore, handlers []Handler) *Reconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}

	return &Reconciler{
		log:      log,
		cfg:      cfg,
		store:    store,
		handlers: handlers,
	}
}

var _ reconcile.Reconciler = (*Reconciler)(nil)

func (r *Reconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	r.client = mgr.GetClient()

	if idx := mgr.GetFieldIndexer(); idx != nil {
		for _, getter := range []indexer.IndexGetter{
			indexer.IndexNodeByTaintKey,
			indexer.IndexGPUDeviceByPoolRefName,
			indexer.IndexGPUDeviceByNamespacedAssignment,
			indexer.IndexGPUDeviceByClusterAssignment,
			indexer.IndexGPUPoolByName,
		} {
			obj, field, extract := getter()
			if err := idx.IndexField(ctx, obj, field, extract); err != nil {
				return err
			}
		}
	}

	c := mgr.GetCache()
	if c == nil {
		return fmt.Errorf("manager cache is required")
	}

	if err := ctr.Watch(
		source.Kind(c, &v1alpha1.GPUPool{}, &handler.TypedEnqueueRequestForObject[*v1alpha1.GPUPool]{}, watcher.PoolPredicates()),
	); err != nil {
		return fmt.Errorf("error setting watch on GPUPool: %w", err)
	}

	for _, w := range []Watcher{
		watchers.NewGPUPoolModuleConfigWatcher(r.log.WithName("watcher.moduleconfig"), r.store),
		watchers.NewGPUPoolGPUDeviceWatcher(r.log.WithName("watcher.device")),
		watchers.NewGPUPoolValidatorPodWatcher(r.log.WithName("watcher.validatorPod")),
	} {
		if err := w.Watch(mgr, ctr); err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("pool", req.Name)
	ctx = logr.NewContext(ctx, log)

	pool := &v1alpha1.GPUPool{}
	pool, err := commonobject.FetchObject(ctx, req.NamespacedName, r.client, pool)
	if err != nil {
		return reconcile.Result{}, err
	}
	if pool == nil {
		log.V(2).Info("GPUPool removed")
		return reconcile.Result{}, nil
	}

	resource := ctrlreconciler.NewResource(pool, r.client)
	s := gpstate.New(r.client, pool)

	rec := ctrlreconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler Handler) (reconcile.Result, error) {
		return handler.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return resource.PatchStatus(ctx)
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return reconcile.Result{}, err
	}

	return res, nil
}
