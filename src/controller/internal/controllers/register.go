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

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/bootstrap"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/clustergpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/gpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/inventory"
	moduleconfigctrl "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/poolusage"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type setupController interface {
	SetupWithManager(context.Context, ctrl.Manager) error
}

var (
	newModuleConfigController = func(log logr.Logger, store *config.ModuleConfigStore) (setupController, error) {
		return moduleconfigctrl.New(log, store)
	}
	newInventoryController = func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.InventoryHandler) (setupController, error) {
		return inventory.New(log, cfg, store, handlers)
	}
	newBootstrapController = func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.BootstrapHandler) (setupController, error) {
		return bootstrap.New(log, cfg, store, handlers), nil
	}
	newPoolController = func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.PoolHandler) (setupController, error) {
		return gpupool.New(log, cfg, store, handlers), nil
	}
	newClusterPoolController  func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.PoolHandler) (setupController, error)
	newPoolUsageController    func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore) (setupController, error)
	newClusterUsageController func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore) (setupController, error)
)

type Dependencies struct {
	Logger            logr.Logger
	InventoryHandlers *contracts.InventoryRegistry
	BootstrapHandlers *contracts.BootstrapRegistry
	PoolHandlers      *contracts.PoolRegistry
	AdmissionHandlers *contracts.AdmissionRegistry
	ModuleConfigStore *config.ModuleConfigStore
}

func ensureRegistries(deps *Dependencies) {
	if newClusterPoolController == nil {
		newClusterPoolController = func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.PoolHandler) (setupController, error) {
			return clustergpupool.New(log, cfg, store, handlers), nil
		}
	}
	if newPoolUsageController == nil {
		newPoolUsageController = func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore) (setupController, error) {
			return poolusage.NewGPUPoolUsage(log, cfg, store), nil
		}
	}
	if newClusterUsageController == nil {
		newClusterUsageController = func(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore) (setupController, error) {
			return poolusage.NewClusterGPUPoolUsage(log, cfg, store), nil
		}
	}
	if deps.InventoryHandlers == nil {
		deps.InventoryHandlers = contracts.NewInventoryRegistry()
	}
	if deps.BootstrapHandlers == nil {
		deps.BootstrapHandlers = contracts.NewBootstrapRegistry()
	}
	if deps.PoolHandlers == nil {
		deps.PoolHandlers = contracts.NewPoolRegistry()
	}
	if deps.AdmissionHandlers == nil {
		deps.AdmissionHandlers = contracts.NewAdmissionRegistry()
	}
}

func Register(ctx context.Context, mgr ctrl.Manager, cfg config.ControllersConfig, store *config.ModuleConfigStore, deps Dependencies) error {
	ensureRegistries(&deps)
	if deps.ModuleConfigStore == nil {
		deps.ModuleConfigStore = store
	}

	constructors := []func() (setupController, error){
		func() (setupController, error) {
			return newModuleConfigController(deps.Logger.WithName("moduleconfig"), deps.ModuleConfigStore)
		},
		func() (setupController, error) {
			return newInventoryController(deps.Logger.WithName("inventory"), cfg.GPUInventory, deps.ModuleConfigStore, deps.InventoryHandlers.List())
		},
		func() (setupController, error) {
			return newBootstrapController(deps.Logger.WithName("bootstrap"), cfg.GPUBootstrap, deps.ModuleConfigStore, deps.BootstrapHandlers.List())
		},
		func() (setupController, error) {
			return newPoolController(deps.Logger.WithName("gpupool"), cfg.GPUPool, deps.ModuleConfigStore, deps.PoolHandlers.List())
		},
		func() (setupController, error) {
			return newClusterPoolController(deps.Logger.WithName("cluster-gpupool"), cfg.GPUPool, deps.ModuleConfigStore, deps.PoolHandlers.List())
		},
		func() (setupController, error) {
			return newPoolUsageController(deps.Logger.WithName("gpupool.usage"), cfg.GPUPool, deps.ModuleConfigStore)
		},
		func() (setupController, error) {
			return newClusterUsageController(deps.Logger.WithName("cluster-gpupool.usage"), cfg.GPUPool, deps.ModuleConfigStore)
		},
	}

	for _, ctor := range constructors {
		controller, err := ctor()
		if err != nil {
			return err
		}
		if err := controller.SetupWithManager(ctx, mgr); err != nil {
			return err
		}
	}

	return nil
}
