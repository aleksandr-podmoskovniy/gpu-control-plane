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

package watcher

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
)

func GPUWorkloadPodPredicates(scope string) predicate.TypedPredicate[*corev1.Pod] {
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
