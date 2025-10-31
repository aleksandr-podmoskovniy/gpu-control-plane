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
	deps.Bootstrap.Register(bootstrap.NewNodeReadinessHandler(log.WithName("bootstrap.node-readiness")))
	deps.Pool.Register(gpupool.NewCapacitySyncHandler(log.WithName("gpupool.capacity-sync")))
	deps.Admission.Register(admission.NewPoolSnapshotHandler(log.WithName("admission.pool-snapshot")))
}

// Handlers groups registry pointers used by controllers.
type Handlers struct {
	Inventory *contracts.InventoryRegistry
	Bootstrap *contracts.BootstrapRegistry
	Pool      *contracts.PoolRegistry
	Admission *contracts.AdmissionRegistry
}
