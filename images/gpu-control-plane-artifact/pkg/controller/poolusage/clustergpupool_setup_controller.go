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

package poolusage

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
	puwatcher "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/poolusage/internal/watcher"
)

func (r *ClusterGPUPoolUsageReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	r.client = mgr.GetClient()

	c := mgr.GetCache()
	if c == nil {
		return fmt.Errorf("manager cache is required")
	}

	if err := ctr.Watch(
		source.Kind(c, &v1alpha1.ClusterGPUPool{}, &handler.TypedEnqueueRequestForObject[*v1alpha1.ClusterGPUPool]{}, puwatcher.ClusterGPUPoolPredicates()),
	); err != nil {
		return fmt.Errorf("error setting watch on ClusterGPUPool: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(
			c,
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(puwatcher.MapPodToClusterPool),
			puwatcher.GPUWorkloadPodPredicates(podlabels.PoolScopeCluster),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on GPU workload Pods: %w", err)
	}

	return nil
}
