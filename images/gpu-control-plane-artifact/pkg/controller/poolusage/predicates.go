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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
)

func namespacedPoolPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPool, _ := e.ObjectOld.(*v1alpha1.GPUPool)
			newPool, _ := e.ObjectNew.(*v1alpha1.GPUPool)
			if oldPool == nil || newPool == nil {
				return true
			}
			if oldPool.Generation != newPool.Generation {
				return true
			}
			if oldPool.Status.Capacity.Total != newPool.Status.Capacity.Total {
				return true
			}
			return false
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func clusterPoolPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPool, _ := e.ObjectOld.(*v1alpha1.ClusterGPUPool)
			newPool, _ := e.ObjectNew.(*v1alpha1.ClusterGPUPool)
			if oldPool == nil || newPool == nil {
				return true
			}
			if oldPool.Generation != newPool.Generation {
				return true
			}
			if oldPool.Status.Capacity.Total != newPool.Status.Capacity.Total {
				return true
			}
			return false
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func gpuWorkloadPodPredicates(scope string) predicate.TypedPredicate[*corev1.Pod] {
	return predicate.TypedFuncs[*corev1.Pod]{
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool {
			return isGPUWorkloadPod(e.Object, scope)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
			oldPod, newPod := e.ObjectOld, e.ObjectNew
			if oldPod == nil || newPod == nil {
				return true
			}
			if !isGPUWorkloadPod(newPod, scope) {
				return false
			}
			if !isGPUWorkloadPod(oldPod, scope) {
				return true
			}
			if oldPod.Spec.NodeName != newPod.Spec.NodeName {
				return true
			}
			if oldPod.Status.Phase != newPod.Status.Phase {
				return true
			}
			if (oldPod.DeletionTimestamp == nil) != (newPod.DeletionTimestamp == nil) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Pod]) bool {
			return isGPUWorkloadPod(e.Object, scope)
		},
		GenericFunc: func(event.TypedGenericEvent[*corev1.Pod]) bool { return false },
	}
}

func isGPUWorkloadPod(pod *corev1.Pod, scope string) bool {
	if pod == nil || pod.Labels == nil {
		return false
	}
	poolName := strings.TrimSpace(pod.Labels[podlabels.PoolNameKey])
	if poolName == "" {
		return false
	}
	return pod.Labels[podlabels.PoolScopeKey] == scope
}
