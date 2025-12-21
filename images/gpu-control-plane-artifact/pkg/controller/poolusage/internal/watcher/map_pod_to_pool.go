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
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
)

func MapPodToNamespacedPool(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil || pod.Labels == nil {
		return nil
	}
	if pod.Labels[podlabels.PoolScopeKey] != podlabels.PoolScopeNamespaced {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels[podlabels.PoolNameKey])
	if poolName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: poolName}}}
}

func MapPodToClusterPool(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil || pod.Labels == nil {
		return nil
	}
	if pod.Labels[podlabels.PoolScopeKey] != podlabels.PoolScopeCluster {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels[podlabels.PoolNameKey])
	if poolName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: poolName}}}
}
