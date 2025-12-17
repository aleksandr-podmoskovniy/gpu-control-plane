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

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool"
	pooladmission "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/admission"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

func SetupController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("cluster-gpupool")

	client := mgr.GetClient()
	renderCfg := pool.RenderConfig{}
	if store != nil {
		state := store.Current()
		renderCfg.CustomTolerationKeys = state.Settings.Placement.CustomTolerationKeys
	}

	handlers := []contracts.PoolHandler{
		pool.NewCompatibilityCheckHandler(),
		pool.NewConfigCheckHandler(client),
		pool.NewSelectionSyncHandler(baseLog.WithName("selection-sync"), client),
		pool.NewNodeMarkHandler(baseLog.WithName("node-mark"), client),
		pool.NewRendererHandler(baseLog.WithName("renderer"), client, renderCfg),
		pool.NewDPValidationHandler(baseLog.WithName("dp-validation"), client),
	}

	r := New(baseLog, cfg, store, handlers)
	if err := r.SetupWithManager(ctx, mgr); err != nil {
		return err
	}

	if mgr.GetWebhookServer() != nil {
		admissionHandlers := []contracts.AdmissionHandler{
			pooladmission.NewPoolValidationHandler(baseLog.WithName("admission")),
		}

		if err := builder.WebhookManagedBy(mgr).
			For(&v1alpha1.ClusterGPUPool{}).
			WithValidator(NewClusterGPUPoolValidator(baseLog, client, admissionHandlers)).
			WithDefaulter(NewClusterGPUPoolDefaulter(baseLog, admissionHandlers)).
			Complete(); err != nil {
			return err
		}
	}

	baseLog.Info("Initialized ClusterGPUPool controller")
	return nil
}
