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

package watchers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func NewGPUPoolModuleConfigWatcher(log logr.Logger, store *moduleconfig.ModuleConfigStore) *ModuleConfigWatcher {
	return NewModuleConfigWatcher(log, store, "list GPUPool to resync after module config change", func(ctx context.Context, cl client.Client, _ *unstructured.Unstructured) ([]reconcile.Request, error) {
		list := &v1alpha1.GPUPoolList{}
		if err := cl.List(ctx, list); err != nil {
			return nil, err
		}

		reqs := make([]reconcile.Request, 0, len(list.Items))
		for _, pool := range list.Items {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: pool.Namespace,
					Name:      pool.Name,
				},
			})
		}

		return reqs, nil
	})
}

func NewClusterGPUPoolModuleConfigWatcher(log logr.Logger, store *moduleconfig.ModuleConfigStore) *ModuleConfigWatcher {
	return NewModuleConfigWatcher(log, store, "list ClusterGPUPool to resync after module config change", func(ctx context.Context, cl client.Client, _ *unstructured.Unstructured) ([]reconcile.Request, error) {
		list := &v1alpha1.ClusterGPUPoolList{}
		if err := cl.List(ctx, list); err != nil {
			return nil, err
		}

		reqs := make([]reconcile.Request, 0, len(list.Items))
		for _, pool := range list.Items {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: pool.Name,
				},
			})
		}

		return reqs, nil
	})
}

func NewGPUNodeStateModuleConfigWatcher(log logr.Logger, store *moduleconfig.ModuleConfigStore) *ModuleConfigWatcher {
	return NewModuleConfigWatcher(log, store, "list GPUNodeState to resync after module config change", func(ctx context.Context, cl client.Client, _ *unstructured.Unstructured) ([]reconcile.Request, error) {
		list := &v1alpha1.GPUNodeStateList{}
		if err := cl.List(ctx, list); err != nil {
			return nil, err
		}

		reqs := make([]reconcile.Request, 0, len(list.Items))
		for _, item := range list.Items {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: item.Name,
				},
			})
		}

		return reqs, nil
	})
}
