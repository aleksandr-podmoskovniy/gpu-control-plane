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
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/admission"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/bootstrap"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/gpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/inventory"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type setupController interface {
	SetupWithManager(context.Context, ctrl.Manager) error
}

var (
	newInventoryController = func(log logr.Logger, cfg config.ControllerConfig, module config.ModuleSettings, handlers []contracts.InventoryHandler) (setupController, error) {
		return inventory.New(log, cfg, module, handlers)
	}
	newBootstrapController = func(log logr.Logger, cfg config.ControllerConfig, handlers []contracts.BootstrapHandler) (setupController, error) {
		return bootstrap.New(log, cfg, handlers), nil
	}
	newPoolController = func(log logr.Logger, cfg config.ControllerConfig, handlers []contracts.PoolHandler) (setupController, error) {
		return gpupool.New(log, cfg, handlers), nil
	}
	newAdmissionController = func(log logr.Logger, cfg config.ControllerConfig, handlers []contracts.AdmissionHandler) (setupController, error) {
		return admission.New(log, cfg, handlers), nil
	}
)

type Dependencies struct {
	Logger            logr.Logger
	InventoryHandlers *contracts.InventoryRegistry
	BootstrapHandlers *contracts.BootstrapRegistry
	PoolHandlers      *contracts.PoolRegistry
	AdmissionHandlers *contracts.AdmissionRegistry
}

func ensureRegistries(deps *Dependencies) {
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

func Register(ctx context.Context, mgr ctrl.Manager, cfg config.ControllersConfig, module config.ModuleSettings, deps Dependencies) error {
	ensureRegistries(&deps)

	constructors := []func() (setupController, error){
		func() (setupController, error) {
			return newInventoryController(deps.Logger.WithName("inventory"), cfg.GPUInventory, module, deps.InventoryHandlers.List())
		},
		func() (setupController, error) {
			return newBootstrapController(deps.Logger.WithName("bootstrap"), cfg.GPUBootstrap, deps.BootstrapHandlers.List())
		},
		func() (setupController, error) {
			return newPoolController(deps.Logger.WithName("gpupool"), cfg.GPUPool, deps.PoolHandlers.List())
		},
		func() (setupController, error) {
			return newAdmissionController(deps.Logger.WithName("admission"), cfg.Admission, deps.AdmissionHandlers.List())
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
