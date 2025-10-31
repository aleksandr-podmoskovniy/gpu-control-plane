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

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// MaintenanceHandler forces requeue while pool is in maintenance mode.
type MaintenanceHandler struct {
	log logr.Logger
}

func NewMaintenanceHandler(log logr.Logger) *MaintenanceHandler {
	return &MaintenanceHandler{log: log}
}

func (h *MaintenanceHandler) Name() string {
	return "maintenance"
}

func (h *MaintenanceHandler) HandlePool(_ context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	for _, cond := range pool.Status.Conditions {
		if cond.Type == "Maintenance" && cond.Status == "True" {
			h.log.V(1).Info("pool maintenance in progress", "pool", pool.Name)
			return contracts.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}
	}
	return contracts.Result{}, nil
}
