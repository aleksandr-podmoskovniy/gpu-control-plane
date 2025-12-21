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

	"github.com/go-logr/logr"
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

type GPUDeviceWatcher struct {
	log logr.Logger
}

func NewGPUDeviceWatcher(log logr.Logger) *GPUDeviceWatcher {
	return &GPUDeviceWatcher{log: log}
}

func (w *GPUDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(
			cache,
			&v1alpha1.GPUDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			devicePredicates(),
		),
	)
}

func (w *GPUDeviceWatcher) enqueue(_ context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	nodeName := strings.TrimSpace(dev.Status.NodeName)
	if nodeName == "" && dev.Labels != nil {
		nodeName = strings.TrimSpace(dev.Labels["gpu.deckhouse.io/node"])
	}
	if nodeName == "" && dev.Labels != nil {
		nodeName = strings.TrimSpace(dev.Labels["kubernetes.io/hostname"])
	}
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func devicePredicates() predicate.TypedPredicate[*v1alpha1.GPUDevice] {
	return predicate.TypedFuncs[*v1alpha1.GPUDevice]{
		CreateFunc:  func(event.TypedCreateEvent[*v1alpha1.GPUDevice]) bool { return true },
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUDevice]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUDevice]) bool { return false },
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.GPUDevice]) bool {
			oldDev, newDev := e.ObjectOld, e.ObjectNew
			if oldDev == nil || newDev == nil {
				return true
			}
			return deviceChanged(oldDev, newDev)
		},
	}
}

func deviceChanged(oldDev, newDev *v1alpha1.GPUDevice) bool {
	if oldDev.Status.State != newDev.Status.State || oldDev.Status.NodeName != newDev.Status.NodeName || oldDev.Status.Managed != newDev.Status.Managed {
		return true
	}
	if (oldDev.Status.PoolRef == nil) != (newDev.Status.PoolRef == nil) {
		return true
	}
	if oldDev.Status.PoolRef != nil && newDev.Status.PoolRef != nil {
		if oldDev.Status.PoolRef.Name != newDev.Status.PoolRef.Name || oldDev.Status.PoolRef.Namespace != newDev.Status.PoolRef.Namespace {
			return true
		}
	}
	return false
}
