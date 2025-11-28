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

package handlers

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/admission"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/bootstrap"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/gpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/inventory"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// RegisterDefaults adds built-in handlers to the provided registries.
func RegisterDefaults(log logr.Logger, deps *Handlers) {
	if deps.Inventory == nil {
		deps.Inventory = contracts.NewInventoryRegistry()
	}
	if deps.Bootstrap == nil {
		deps.Bootstrap = contracts.NewBootstrapRegistry()
	}
	if deps.Pool == nil {
		deps.Pool = contracts.NewPoolRegistry()
	}
	if deps.Admission == nil {
		deps.Admission = contracts.NewAdmissionRegistry()
	}

	deps.Inventory.Register(inventory.NewDeviceStateHandler(log.WithName("inventory.device-state")))
	deps.Bootstrap.Register(bootstrap.NewWorkloadStatusHandler(log.WithName("bootstrap.workload-status"), ""))
	deps.Bootstrap.Register(bootstrap.NewDeviceStateSyncHandler(log.WithName("bootstrap.device-state-sync")))
	deps.Bootstrap.Register(bootstrap.NewNodeReadinessHandler(log.WithName("bootstrap.node-readiness")))
	deps.Pool.Register(gpupool.NewCompatibilityCheckHandler())
	if deps.Client != nil {
		deps.Pool.Register(gpupool.NewConfigCheckHandler(deps.Client))
		deps.Pool.Register(gpupool.NewSelectionSyncHandler(log.WithName("gpupool.selection-sync"), deps.Client))
		deps.Pool.Register(gpupool.NewNodeMarkHandler(log.WithName("gpupool.node-mark"), deps.Client))
	}
	deps.Pool.Register(gpupool.NewCapacitySyncHandler(log.WithName("gpupool.capacity-sync")))
	if deps.Client != nil {
		renderCfg := gpupool.RenderConfig{}
		if deps.ModuleConfigStore != nil {
			state := deps.ModuleConfigStore.Current()
			renderCfg.CustomTolerationKeys = state.Settings.Placement.CustomTolerationKeys
		}
		deps.Pool.Register(gpupool.NewRendererHandler(log.WithName("gpupool.renderer"), deps.Client, renderCfg))
	}
	deps.Admission.Register(admission.NewPoolValidationHandler(log.WithName("admission.pool-validation")))
	deps.Admission.Register(admission.NewPoolSnapshotHandler(log.WithName("admission.pool-snapshot")))
}

// Handlers groups registry pointers used by controllers.
type Handlers struct {
	Inventory         *contracts.InventoryRegistry
	Bootstrap         *contracts.BootstrapRegistry
	Pool              *contracts.PoolRegistry
	Admission         *contracts.AdmissionRegistry
	Client            client.Client
	ModuleConfigStore *config.ModuleConfigStore
}
