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

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
)

type GPUPoolGPUDeviceWatcher struct {
	log      logr.Logger
	enqueuer *GPUPoolGPUDeviceEnqueuer
}

func NewGPUPoolGPUDeviceWatcher(log logr.Logger) *GPUPoolGPUDeviceWatcher {
	return &GPUPoolGPUDeviceWatcher{
		log:      log,
		enqueuer: NewGPUPoolGPUDeviceEnqueuer(log, nil),
	}
}

func (w *GPUPoolGPUDeviceWatcher) enqueue(ctx context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if w.enqueuer == nil {
		w.enqueuer = NewGPUPoolGPUDeviceEnqueuer(w.log, nil)
	}
	return w.enqueuer.EnqueueRequests(ctx, dev)
}

func (w *GPUPoolGPUDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	w.enqueuer = NewGPUPoolGPUDeviceEnqueuer(w.log, mgr.GetClient())

	return ctr.Watch(
		source.Kind(
			cache,
			&v1alpha1.GPUDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			NewGPUDeviceFilter(commonannotations.GPUDeviceAssignment).Predicates(),
		),
	)
}

type ClusterGPUPoolGPUDeviceWatcher struct {
	log      logr.Logger
	enqueuer *ClusterGPUPoolGPUDeviceEnqueuer
}

func NewClusterGPUPoolGPUDeviceWatcher(log logr.Logger) *ClusterGPUPoolGPUDeviceWatcher {
	return &ClusterGPUPoolGPUDeviceWatcher{
		log:      log,
		enqueuer: NewClusterGPUPoolGPUDeviceEnqueuer(),
	}
}

func (w *ClusterGPUPoolGPUDeviceWatcher) enqueue(ctx context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if w.enqueuer == nil {
		w.enqueuer = NewClusterGPUPoolGPUDeviceEnqueuer()
	}
	return w.enqueuer.EnqueueRequests(ctx, dev)
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
			NewGPUDeviceFilter(commonannotations.ClusterGPUDeviceAssignment).Predicates(),
		),
	)
}
