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

package bootstrap

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type Reconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	cfg      config.ControllerConfig
	handlers []contracts.BootstrapHandler
}

func New(log logr.Logger, cfg config.ControllerConfig, handlers []contracts.BootstrapHandler) *Reconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	return &Reconciler{
		log:      log,
		cfg:      cfg,
		handlers: handlers,
	}
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()

	return ctrl.NewControllerManagedBy(mgr).
		Named("gpu-bootstrap-controller").
		For(&gpuv1alpha1.GPUNodeInventory{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.cfg.Workers}).
		Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: req.Name}, inventory); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.V(2).Info("GPUNodeInventory removed", "name", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	aggregate := contracts.Result{}
	for _, handler := range r.handlers {
		res, err := handler.HandleNode(ctx, inventory)
		if err != nil {
			r.log.Error(err, "bootstrap handler failed", "handler", handler.Name(), "node", req.Name)
			return ctrl.Result{}, err
		}
		aggregate = contracts.MergeResult(aggregate, res)
	}

	result := ctrl.Result{}
	if aggregate.Requeue {
		result.Requeue = true
	}
	if aggregate.RequeueAfter > 0 {
		result.RequeueAfter = aggregate.RequeueAfter
	}

	return result, nil
}
