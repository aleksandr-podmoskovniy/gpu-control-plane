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

	inv, err := inventory.New(deps.Logger.WithName("inventory"), cfg.GPUInventory, module, deps.InventoryHandlers.List())
	if err != nil {
		return err
	}
	if err := inv.SetupWithManager(ctx, mgr); err != nil {
		return err
	}
	if err := bootstrap.New(deps.Logger.WithName("bootstrap"), cfg.GPUBootstrap, deps.BootstrapHandlers.List()).SetupWithManager(ctx, mgr); err != nil {
		return err
	}
	if err := gpupool.New(deps.Logger.WithName("gpupool"), cfg.GPUPool, deps.PoolHandlers.List()).SetupWithManager(ctx, mgr); err != nil {
		return err
	}
	if err := admission.New(deps.Logger.WithName("admission"), cfg.Admission, deps.AdmissionHandlers.List()).SetupWithManager(ctx, mgr); err != nil {
		return err
	}
	return nil
}
