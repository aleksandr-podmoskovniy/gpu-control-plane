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
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func PoolPredicates() predicate.TypedPredicate[*v1alpha1.ClusterGPUPool] {
	return predicate.TypedFuncs[*v1alpha1.ClusterGPUPool]{
		CreateFunc: func(event.TypedCreateEvent[*v1alpha1.ClusterGPUPool]) bool { return true },
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.ClusterGPUPool]) bool {
			oldPool := e.ObjectOld
			newPool := e.ObjectNew
			if oldPool == nil || newPool == nil {
				return true
			}
			return !poolSpecEqual(oldPool.Spec, newPool.Spec)
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.ClusterGPUPool]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.ClusterGPUPool]) bool { return false },
	}
}

func poolSpecEqual(a, b v1alpha1.GPUPoolSpec) bool {
	return reflect.DeepEqual(a, b)
}

