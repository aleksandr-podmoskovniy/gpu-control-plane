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
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	common "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common"
)

type WorkloadPodWatcher struct {
	log           logr.Logger
	managedAppSet map[string]struct{}
}

func NewWorkloadPodWatcher(log logr.Logger) *WorkloadPodWatcher {
	set := make(map[string]struct{})
	for _, name := range common.ComponentAppNames() {
		set[name] = struct{}{}
	}
	return &WorkloadPodWatcher{log: log, managedAppSet: set}
}

func (w *WorkloadPodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(cache, &corev1.Pod{}, handler.TypedEnqueueRequestsFromMapFunc(w.enqueue)),
	)
}

func (w *WorkloadPodWatcher) enqueue(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil {
		return nil
	}
	if pod.Namespace != common.WorkloadsNamespace {
		return nil
	}
	if pod.Spec.NodeName == "" {
		return nil
	}
	if pod.Labels == nil {
		return nil
	}
	if _, ok := w.managedAppSet[pod.Labels["app"]]; !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: pod.Spec.NodeName}}}
}
