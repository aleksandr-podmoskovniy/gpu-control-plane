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
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
)

type controllerBuilder interface {
	Named(string) controllerBuilder
	For(client.Object, ...builder.ForOption) controllerBuilder
	WithOptions(controller.Options) controllerBuilder
	Complete(reconcile.Reconciler) error
}

type runtimeControllerBuilder struct {
	builder *builder.Builder
}

func (b *runtimeControllerBuilder) Named(name string) controllerBuilder {
	b.builder = b.builder.Named(name)
	return b
}

func (b *runtimeControllerBuilder) For(obj client.Object, opts ...builder.ForOption) controllerBuilder {
	b.builder = b.builder.For(obj, opts...)
	return b
}

func (b *runtimeControllerBuilder) WithOptions(opts controller.Options) controllerBuilder {
	b.builder = b.builder.WithOptions(opts)
	return b
}

func (b *runtimeControllerBuilder) Complete(r reconcile.Reconciler) error {
	return b.builder.Complete(r)
}

const (
	controllerName           = "gpu-pool-controller"
	cacheSyncTimeoutDuration = 10 * time.Minute
)

var newControllerManagedBy = func(mgr ctrl.Manager) controllerBuilder {
	return &runtimeControllerBuilder{builder: ctrl.NewControllerManagedBy(mgr)}
}

type Reconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	cfg      config.ControllerConfig
	handlers []contracts.PoolHandler
	builders func(ctrl.Manager) controllerBuilder
}

func New(log logr.Logger, cfg config.ControllerConfig, handlers []contracts.PoolHandler) *Reconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}

	return &Reconciler{
		log:      log,
		cfg:      cfg,
		handlers: handlers,
		builders: newControllerManagedBy,
	}
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
		For(&gpuv1alpha1.GPUPool{}).
		WithOptions(options)
	return builder.Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("pool", req.Name)
	ctx = logr.NewContext(ctx, log)

	pool := &gpuv1alpha1.GPUPool{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: req.Name}, pool); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("GPUPool removed")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.PoolHandler) (contracts.Result, error) {
		return handler.HandlePool(ctx, pool)
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

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
