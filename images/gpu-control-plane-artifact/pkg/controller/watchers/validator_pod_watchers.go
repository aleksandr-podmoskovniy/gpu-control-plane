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
	corev1 "k8s.io/api/core/v1"
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
	commonpod "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/pod"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

type GPUPoolValidatorPodWatcher struct {
	log logr.Logger
	cl  client.Client
}

func NewGPUPoolValidatorPodWatcher(log logr.Logger) *GPUPoolValidatorPodWatcher {
	return &GPUPoolValidatorPodWatcher{log: log}
}

func (w *GPUPoolValidatorPodWatcher) enqueue(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	if !isValidatorPoolPod(pod) {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels["pool"])

	cl := w.cl
	if cl == nil {
		return nil
	}

	list := &v1alpha1.GPUPoolList{}
	if err := cl.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: poolName}); err != nil {
		if w.log.GetSink() != nil {
			w.log.Error(err, "list GPUPool by name to map validator pod event", "pod", pod.Name, "pool", poolName)
		}
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		pool := list.Items[i]
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name},
		})
	}
	return reqs
}

func (w *GPUPoolValidatorPodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	w.cl = mgr.GetClient()

	return ctr.Watch(
		source.Kind(
			cache,
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
			validatorPodPredicates(),
		),
	)
}

type ClusterGPUPoolValidatorPodWatcher struct {
	log logr.Logger
}

func NewClusterGPUPoolValidatorPodWatcher(log logr.Logger) *ClusterGPUPoolValidatorPodWatcher {
	return &ClusterGPUPoolValidatorPodWatcher{log: log}
}

func (w *ClusterGPUPoolValidatorPodWatcher) enqueue(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if !isValidatorPoolPod(pod) {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels["pool"])
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: poolName}}}
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
			validatorPodPredicates(),
		),
	)
}

func validatorPodPredicates() predicate.TypedPredicate[*corev1.Pod] {
	return predicate.TypedFuncs[*corev1.Pod]{
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool {
			return isValidatorPoolPod(e.Object)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
			oldPod, newPod := e.ObjectOld, e.ObjectNew
			if oldPod == nil || newPod == nil {
				return true
			}
			if !isValidatorPoolPod(newPod) {
				return false
			}
			if !isValidatorPoolPod(oldPod) {
				return true
			}
			if oldPod.Spec.NodeName != newPod.Spec.NodeName {
				return true
			}
			return commonpod.IsReady(oldPod) != commonpod.IsReady(newPod)
		},
		DeleteFunc:  func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return isValidatorPoolPod(e.Object) },
		GenericFunc: func(event.TypedGenericEvent[*corev1.Pod]) bool { return false },
	}
}

func isValidatorPoolPod(pod *corev1.Pod) bool {
	if pod == nil || pod.Labels == nil {
		return false
	}
	if pod.Labels["app"] != "nvidia-operator-validator" {
		return false
	}
	return strings.TrimSpace(pod.Labels["pool"]) != ""
}
