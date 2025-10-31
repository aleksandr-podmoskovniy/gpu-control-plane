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

	"github.com/go-logr/logr"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// CapacitySyncHandler recalculates pool capacity based on the status devices list.
type CapacitySyncHandler struct {
	log logr.Logger
}

func NewCapacitySyncHandler(log logr.Logger) *CapacitySyncHandler {
	return &CapacitySyncHandler{log: log}
}

func (h *CapacitySyncHandler) Name() string {
	return "capacity-sync"
}

func (h *CapacitySyncHandler) HandlePool(_ context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	var total, used int32
	for _, node := range pool.Status.Nodes {
		total += node.TotalDevices
		used += node.AssignedDevices
	}

	pool.Status.Capacity.Total = total
	pool.Status.Capacity.Used = used
	if total >= used {
		pool.Status.Capacity.Available = total - used
	}

	h.log.V(2).Info("synchronised pool capacity", "pool", pool.Name, "total", total, "used", used)
	return contracts.Result{}, nil
}
