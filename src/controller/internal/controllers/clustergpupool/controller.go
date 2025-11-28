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

package clustergpupool

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfigctrl "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
)

const (
	controllerName           = "cluster-gpu-pool-controller"
	cacheSyncTimeoutDuration = 10 * time.Minute
)

type controllerBuilder interface {
	Named(string) controllerBuilder
	For(client.Object, ...builder.ForOption) controllerBuilder
	WithOptions(controller.Options) controllerBuilder
	WatchesRawSource(source.Source) controllerBuilder
	Complete(reconcile.Reconciler) error
}

type controllerRuntimeAdapter interface {
	Named(string) controllerRuntimeAdapter
	For(client.Object, ...builder.ForOption) controllerRuntimeAdapter
	WithOptions(controller.Options) controllerRuntimeAdapter
	WatchesRawSource(source.Source) controllerRuntimeAdapter
	Complete(reconcile.Reconciler) error
}

type runtimeControllerBuilder struct {
	adapter controllerRuntimeAdapter
}

func (b *runtimeControllerBuilder) Named(name string) controllerBuilder {
	b.adapter = b.adapter.Named(name)
	return b
}

func (b *runtimeControllerBuilder) For(obj client.Object, opts ...builder.ForOption) controllerBuilder {
	b.adapter = b.adapter.For(obj, opts...)
	return b
}

func (b *runtimeControllerBuilder) WithOptions(opts controller.Options) controllerBuilder {
	b.adapter = b.adapter.WithOptions(opts)
	return b
}

func (b *runtimeControllerBuilder) WatchesRawSource(src source.Source) controllerBuilder {
	b.adapter = b.adapter.WatchesRawSource(src)
	return b
}

func (b *runtimeControllerBuilder) Complete(r reconcile.Reconciler) error {
	return b.adapter.Complete(r)
}

type builderDelegate interface {
	Named(string) *builder.Builder
	For(client.Object, ...builder.ForOption) *builder.Builder
	WithOptions(controller.Options) *builder.Builder
	WatchesRawSource(source.Source) *builder.Builder
	Complete(reconcile.Reconciler) error
}

type builderControllerAdapter struct {
	delegate builderDelegate
}

func (a *builderControllerAdapter) Named(name string) controllerRuntimeAdapter {
	a.delegate.Named(name)
	return a
}

func (a *builderControllerAdapter) For(obj client.Object, opts ...builder.ForOption) controllerRuntimeAdapter {
	a.delegate.For(obj, opts...)
	return a
}

func (a *builderControllerAdapter) WithOptions(opts controller.Options) controllerRuntimeAdapter {
	a.delegate.WithOptions(opts)
	return a
}

func (a *builderControllerAdapter) WatchesRawSource(src source.Source) controllerRuntimeAdapter {
	a.delegate.WatchesRawSource(src)
	return a
}

func (a *builderControllerAdapter) Complete(r reconcile.Reconciler) error {
	return a.delegate.Complete(r)
}

var newControllerManagedBy = func(mgr ctrl.Manager) controllerBuilder {
	return &runtimeControllerBuilder{
		adapter: &builderControllerAdapter{delegate: ctrl.NewControllerManagedBy(mgr)},
	}
}

type Reconciler struct {
	client               client.Client
	scheme               *runtime.Scheme
	log                  logr.Logger
	cfg                  config.ControllerConfig
	store                *config.ModuleConfigStore
	handlers             []contracts.PoolHandler
	builders             func(ctrl.Manager) controllerBuilder
	moduleWatcherFactory func(cache.Cache, controllerBuilder) controllerBuilder
}

func New(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.PoolHandler) *Reconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	rec := &Reconciler{
		cfg:      cfg,
		log:      log,
		store:    store,
		handlers: handlers,
		builders: newControllerManagedBy,
	}
	rec.moduleWatcherFactory = func(c cache.Cache, builder controllerBuilder) controllerBuilder {
		return rec.attachModuleWatcher(builder, c)
	}
	return rec
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()

	options := controller.Options{
		MaxConcurrentReconciles: r.cfg.Workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
		CacheSyncTimeout:        cacheSyncTimeoutDuration,
	}

	builder := r.builders(mgr).
		Named(controllerName).
		For(&v1alpha1.ClusterGPUPool{}).
		WithOptions(options)

	if cache := mgr.GetCache(); r.moduleWatcherFactory != nil && cache != nil {
		builder = r.moduleWatcherFactory(cache, builder)
	}

	return builder.Complete(r)
}

func (r *Reconciler) attachModuleWatcher(builder controllerBuilder, c cache.Cache) controllerBuilder {
	moduleConfig := &unstructured.Unstructured{}
	moduleConfig.SetGroupVersionKind(moduleconfigctrl.ModuleConfigGVK)
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc(r.mapModuleConfig)
	return builder.WatchesRawSource(source.Kind(c, moduleConfig, handlerFunc))
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
	return r.requeueAllPools(ctx)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("clusterPool", req.Name)
	ctx = logr.NewContext(ctx, log)

	clusterPool := &v1alpha1.ClusterGPUPool{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: req.Name}, clusterPool); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("ClusterGPUPool removed")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Reuse existing pool handlers by working with a GPUPool-shaped object.
	pool := &v1alpha1.GPUPool{
		TypeMeta:   clusterPool.TypeMeta,
		ObjectMeta: clusterPool.ObjectMeta,
		Spec:       clusterPool.Spec,
		Status:     clusterPool.Status,
	}

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.PoolHandler) (contracts.Result, error) {
		return handler.HandlePool(ctx, pool)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		clusterPool.Status = pool.Status
		return r.client.Status().Update(ctx, clusterPool)
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return ctrl.Result{}, err
	}

	return ctrl.Result{
		Requeue:      res.Requeue,
		RequeueAfter: res.RequeueAfter,
	}, nil
}

func (r *Reconciler) requeueAllPools(ctx context.Context) []reconcile.Request {
	list := &v1alpha1.ClusterGPUPoolList{}
	if err := r.client.List(ctx, list); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list ClusterGPUPool to resync after module config change")
		}
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, pool := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: pool.Name}})
	}
	return reqs
}
