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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	gphandler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/gpupool/internal/handler"
	gpwebhook "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/gpupool/internal/webhook"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	pooladmission "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/admission"
	poolcompat "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/compatibility"
	poolconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/config"
	pooldpvalidation "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/dpvalidation"
	poolnodemark "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/nodemark"
	poolselection "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/selection"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
)

const ControllerName = "gpu-pool-controller"

func SetupController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("gpupool")

	client := mgr.GetClient()
	workloadCfg := poolconfig.WorkloadConfig{}
	if store != nil {
		state := store.Current()
		workloadCfg.CustomTolerationKeys = state.Settings.Placement.CustomTolerationKeys
	}

	handlers := []Handler{
		gphandler.WrapPoolHandler(poolcompat.NewCompatibilityCheckHandler()),
		gphandler.WrapPoolHandler(poolconfig.NewConfigCheckHandler(client)),
		gphandler.WrapPoolHandler(poolselection.NewSelectionSyncHandler(baseLog.WithName("selection-sync"), client)),
		gphandler.WrapPoolHandler(poolnodemark.NewNodeMarkHandler(baseLog.WithName("node-mark"), client)),
		gphandler.WrapPoolHandler(gphandler.NewWorkloadHandler(baseLog.WithName("workload"), client, workloadCfg)),
		gphandler.WrapPoolHandler(pooldpvalidation.NewDPValidationHandler(baseLog.WithName("dp-validation"), client)),
	}

	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}

	r := NewReconciler(baseLog, cfg, store, handlers)

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
		admissionHandlers := []gpwebhook.AdmissionHandler{
			pooladmission.NewPoolValidationHandler(baseLog.WithName("admission")),
		}

		if err := builder.WebhookManagedBy(mgr).
			For(&v1alpha1.GPUPool{}).
			WithValidator(gpwebhook.NewGPUPoolValidator(baseLog, client, admissionHandlers)).
			WithDefaulter(gpwebhook.NewGPUPoolDefaulter(baseLog, admissionHandlers)).
			Complete(); err != nil {
			return err
		}

		if err := builder.WebhookManagedBy(mgr).
			For(&corev1.Pod{}).
			WithDefaulter(gpwebhook.NewPodDefaulter(baseLog, store, client)).
			WithValidator(gpwebhook.NewPodValidator(baseLog, client)).
			Complete(); err != nil {
			return err
		}
	}

	baseLog.Info("Initialized GPUPool controller")
	return nil
}
