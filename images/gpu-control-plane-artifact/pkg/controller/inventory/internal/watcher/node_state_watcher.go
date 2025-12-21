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
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type NodeStateWatcher struct{}

func NewNodeStateWatcher() *NodeStateWatcher {
	return &NodeStateWatcher{}
}

func (w *NodeStateWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(
			cache,
			&v1alpha1.GPUNodeState{},
			handler.TypedEnqueueRequestsFromMapFunc(mapNodeStateToNode),
			nodeStatePredicates(),
		),
	)
}

func mapNodeStateToNode(_ context.Context, state *v1alpha1.GPUNodeState) []reconcile.Request {
	if state == nil {
		return nil
	}
	nodeName := strings.TrimSpace(state.Name)
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func nodeStatePredicates() predicate.TypedPredicate[*v1alpha1.GPUNodeState] {
	return predicate.TypedFuncs[*v1alpha1.GPUNodeState]{
		CreateFunc:  func(event.TypedCreateEvent[*v1alpha1.GPUNodeState]) bool { return false },
		UpdateFunc:  func(event.TypedUpdateEvent[*v1alpha1.GPUNodeState]) bool { return false },
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUNodeState]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUNodeState]) bool { return false },
	}
}

