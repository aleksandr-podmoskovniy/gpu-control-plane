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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type GPUPoolValidatorPodWatcher struct {
	log      logr.Logger
	enqueuer *GPUPoolValidatorPodEnqueuer
}

func NewGPUPoolValidatorPodWatcher(log logr.Logger) *GPUPoolValidatorPodWatcher {
	return &GPUPoolValidatorPodWatcher{
		log:      log,
		enqueuer: NewGPUPoolValidatorPodEnqueuer(log, nil),
	}
}

func (w *GPUPoolValidatorPodWatcher) enqueue(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	if w.enqueuer == nil {
		w.enqueuer = NewGPUPoolValidatorPodEnqueuer(w.log, nil)
	}
	return w.enqueuer.EnqueueRequests(ctx, pod)
}

func (w *GPUPoolValidatorPodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	w.enqueuer = NewGPUPoolValidatorPodEnqueuer(w.log, mgr.GetClient())

	return ctr.Watch(
		source.Kind(
			cache,
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			NewValidatorPodFilter().Predicates(),
		),
	)
}

type ClusterGPUPoolValidatorPodWatcher struct {
	log      logr.Logger
	enqueuer *ClusterGPUPoolValidatorPodEnqueuer
}

func NewClusterGPUPoolValidatorPodWatcher(log logr.Logger) *ClusterGPUPoolValidatorPodWatcher {
	return &ClusterGPUPoolValidatorPodWatcher{
		log:      log,
		enqueuer: NewClusterGPUPoolValidatorPodEnqueuer(),
	}
}

func (w *ClusterGPUPoolValidatorPodWatcher) enqueue(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	if w.enqueuer == nil {
		w.enqueuer = NewClusterGPUPoolValidatorPodEnqueuer()
	}
	return w.enqueuer.EnqueueRequests(ctx, pod)
}

func (w *ClusterGPUPoolValidatorPodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(
			cache,
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			NewValidatorPodFilter().Predicates(),
		),
	)
}
