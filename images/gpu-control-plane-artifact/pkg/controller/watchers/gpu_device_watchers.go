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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

type GPUPoolGPUDeviceWatcher struct {
	log logr.Logger
	cl  client.Client
}

func NewGPUPoolGPUDeviceWatcher(log logr.Logger) *GPUPoolGPUDeviceWatcher {
	return &GPUPoolGPUDeviceWatcher{log: log}
}

func (w *GPUPoolGPUDeviceWatcher) enqueue(ctx context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	targetPools := map[string]struct{}{}
	reqSet := map[types.NamespacedName]struct{}{}

	if ref := dev.Status.PoolRef; ref != nil {
		if ref.Name != "" {
			if ref.Namespace != "" {
				reqSet[types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}] = struct{}{}
			} else {
				targetPools[ref.Name] = struct{}{}
			}
		}
	}

	if ann := strings.TrimSpace(dev.Annotations[commonannotations.GPUDeviceAssignment]); ann != "" {
		targetPools[ann] = struct{}{}
	}

	cl := w.cl
	if len(targetPools) == 0 || cl == nil {
		if len(reqSet) == 0 {
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(reqSet))
		for nn := range reqSet {
			reqs = append(reqs, reconcile.Request{NamespacedName: nn})
		}
		return reqs
	}

	for poolName := range targetPools {
		list := &v1alpha1.GPUPoolList{}
		if err := cl.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: poolName}); err != nil {
			if w.log.GetSink() != nil {
				w.log.Error(err, "list GPUPool by name to map device event", "device", dev.Name, "pool", poolName)
			}
			continue
		}
		for i := range list.Items {
			pool := list.Items[i]
			reqSet[types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}] = struct{}{}
		}
	}

	if len(reqSet) == 0 {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(reqSet))
	for nn := range reqSet {
		reqs = append(reqs, reconcile.Request{NamespacedName: nn})
	}
	return reqs
}

func (w *GPUPoolGPUDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	w.cl = mgr.GetClient()

	return ctr.Watch(
		source.Kind(
			cache,
			&v1alpha1.GPUDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			gpuDevicePredicates(commonannotations.GPUDeviceAssignment),
		),
	)
}

type ClusterGPUPoolGPUDeviceWatcher struct {
	log logr.Logger
}

func NewClusterGPUPoolGPUDeviceWatcher(log logr.Logger) *ClusterGPUPoolGPUDeviceWatcher {
	return &ClusterGPUPoolGPUDeviceWatcher{log: log}
}

func (w *ClusterGPUPoolGPUDeviceWatcher) enqueue(_ context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	targets := map[types.NamespacedName]struct{}{}
	if ref := dev.Status.PoolRef; ref != nil {
		if ref.Name != "" && strings.TrimSpace(ref.Namespace) == "" {
			targets[types.NamespacedName{Name: ref.Name}] = struct{}{}
		}
	}
	if ann := strings.TrimSpace(dev.Annotations[commonannotations.ClusterGPUDeviceAssignment]); ann != "" {
		targets[types.NamespacedName{Name: ann}] = struct{}{}
	}
	if len(targets) == 0 {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(targets))
	for nn := range targets {
		reqs = append(reqs, reconcile.Request{NamespacedName: nn})
	}
	return reqs
}

func (w *ClusterGPUPoolGPUDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(
			cache,
			&v1alpha1.GPUDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			gpuDevicePredicates(commonannotations.ClusterGPUDeviceAssignment),
		),
	)
}

func gpuDevicePredicates(assignmentAnnotation string) predicate.TypedPredicate[*v1alpha1.GPUDevice] {
	return predicate.TypedFuncs[*v1alpha1.GPUDevice]{
		CreateFunc: func(e event.TypedCreateEvent[*v1alpha1.GPUDevice]) bool {
			dev := e.Object
			return dev != nil && (strings.TrimSpace(dev.Annotations[assignmentAnnotation]) != "" || dev.Status.PoolRef != nil)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.GPUDevice]) bool {
			oldDev := e.ObjectOld
			newDev := e.ObjectNew
			if oldDev == nil || newDev == nil {
				return true
			}
			return gpuDeviceChanged(oldDev, newDev, assignmentAnnotation)
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUDevice]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUDevice]) bool { return false },
	}
}

func gpuDeviceChanged(oldDev, newDev *v1alpha1.GPUDevice, assignmentAnnotation string) bool {
	if strings.TrimSpace(oldDev.Annotations[assignmentAnnotation]) != strings.TrimSpace(newDev.Annotations[assignmentAnnotation]) {
		return true
	}
	if oldDev.Status.State != newDev.Status.State || oldDev.Status.NodeName != newDev.Status.NodeName {
		return true
	}
	if oldDev.Status.Hardware.UUID != newDev.Status.Hardware.UUID {
		return true
	}
	if !equality.Semantic.DeepEqual(oldDev.Status.Hardware.MIG, newDev.Status.Hardware.MIG) {
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
