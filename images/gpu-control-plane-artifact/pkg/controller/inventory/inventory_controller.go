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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	invinternal "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal"
	invwebhook "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/webhook"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
)

func SetupController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("inventory")
	handlers := []contracts.InventoryHandler{
		invinternal.NewDeviceStateHandler(baseLog.WithName("device-state")),
	}

	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}

	r, err := NewReconciler(baseLog, cfg, store, handlers)
	if err != nil {
		return err
	}

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(baseLog),
		CacheSyncTimeout:        10 * time.Minute,
		NewQueue:                reconciler.NewNamedQueue(reconciler.UsePriorityQueue()),
	})
	if err != nil {
		return err
	}

	if err := r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	if mgr.GetWebhookServer() != nil {
		if err := builder.WebhookManagedBy(mgr).
			For(&v1alpha1.GPUDevice{}).
			WithValidator(invwebhook.NewGPUDeviceAssignmentValidator(baseLog, mgr.GetClient())).
			Complete(); err != nil {
			return err
		}
	}

	baseLog.Info("Initialized GPU inventory controller")
	return nil
}
