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

package handler

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/deps"
	poolworkload "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/workload"
)

// WorkloadHandler reconciles per-pool workloads (device-plugin, validator, MIG manager).
type WorkloadHandler struct {
	deps deps.Deps
}

func NewWorkloadHandler(log logr.Logger, c client.Client, cfg config.WorkloadConfig) *WorkloadHandler {
	return &WorkloadHandler{
		deps: poolworkload.NewDeps(log, c, cfg),
	}
}

func (h *WorkloadHandler) Name() string {
	return "workload"
}

func (h *WorkloadHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (reconcile.Result, error) {
	return poolworkload.Reconcile(ctx, h.deps, pool)
}
