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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type NodeWatcher struct{}

func NewNodeWatcher() *NodeWatcher {
	return &NodeWatcher{}
}

func (w *NodeWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(cache, &corev1.Node{}, &handler.TypedEnqueueRequestForObject[*corev1.Node]{}, nodePredicates()),
	)
}

func nodePredicates() predicate.TypedPredicate[*corev1.Node] {
	return predicate.TypedFuncs[*corev1.Node]{
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Node]) bool {
			node := e.Object
			if node == nil {
				return false
			}
			return hasGPUDeviceLabels(node.GetLabels())
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
			return gpuNodeLabelsChanged(e.ObjectOld, e.ObjectNew)
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*corev1.Node]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*corev1.Node]) bool { return false },
	}
}

func gpuNodeLabelsChanged(oldNode, newNode *corev1.Node) bool {
	oldLabels := nodeLabels(oldNode)
	newLabels := nodeLabels(newNode)

	oldHas := hasGPUDeviceLabels(oldLabels)
	newHas := hasGPUDeviceLabels(newLabels)
	if oldHas != newHas {
		return true
	}

	return gpuLabelsDiffer(oldLabels, newLabels)
}

func nodeLabels(node *corev1.Node) map[string]string {
	if node == nil {
		return nil
	}
	return node.Labels
}
