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

package internal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pustate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/usage/internal/state"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

type GPUPoolHandler interface {
	Name() string
	Handle(ctx context.Context, s pustate.GPUPoolState) (reconcile.Result, error)
}

type gpuPoolUsageHandler struct{}

func NewGPUPoolUsageHandler() GPUPoolHandler {
	return &gpuPoolUsageHandler{}
}

func (h *gpuPoolUsageHandler) Name() string {
	return "pool-usage"
}

func (h *gpuPoolUsageHandler) Handle(ctx context.Context, s pustate.GPUPoolState) (reconcile.Result, error) {
	pool := s.Pool()

	pods := &corev1.PodList{}
	if err := s.Client().List(ctx, pods,
		client.InNamespace(pool.Namespace),
		client.MatchingLabels{
			poolcommon.PoolNameKey:  pool.Name,
			poolcommon.PoolScopeKey: poolcommon.PoolScopeNamespaced,
		}); err != nil {
		return reconcile.Result{}, err
	}

	resourceName := corev1.ResourceName("gpu.deckhouse.io/" + pool.Name)
	var used int64
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !pustate.PodCountsTowardsUsage(pod) {
			continue
		}
		used += pustate.RequestedResources(pod, resourceName)
	}

	s.SetUsed(pustate.ClampInt64ToInt32(used))
	return reconcile.Result{}, nil
}
