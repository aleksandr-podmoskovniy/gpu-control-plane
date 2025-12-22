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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	common "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common"
)

type GFDPodWatcher struct {
	gfdApp string
}

func NewGFDPodWatcher() *GFDPodWatcher {
	return &GFDPodWatcher{gfdApp: common.AppName(common.ComponentGPUFeatureDiscovery)}
}

func (w *GFDPodWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	return ctr.Watch(
		source.Kind(
			cache,
			&corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(mapGFDPodToNode),
			gfdPodPredicates(w.gfdApp),
		),
	)
}

func mapGFDPodToNode(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil {
		return nil
	}
	nodeName := strings.TrimSpace(pod.Spec.NodeName)
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func gfdPodPredicates(gfdApp string) predicate.TypedPredicate[*corev1.Pod] {
	return predicate.TypedFuncs[*corev1.Pod]{
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool {
			pod := e.Object
			return isGFDPod(pod, gfdApp) && pod.Spec.NodeName != "" && pod.Status.PodIP != "" && isPodReady(pod)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
			oldPod, newPod := e.ObjectOld, e.ObjectNew
			if newPod == nil {
				return true
			}
			if !isGFDPod(newPod, gfdApp) {
				return false
			}
			if oldPod == nil || !isGFDPod(oldPod, gfdApp) {
				return true
			}
			if oldPod.Spec.NodeName != newPod.Spec.NodeName {
				return true
			}
			if oldPod.Status.PodIP != newPod.Status.PodIP {
				return true
			}
			return isPodReady(oldPod) != isPodReady(newPod)
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*corev1.Pod]) bool { return false },
		GenericFunc: func(event.TypedGenericEvent[*corev1.Pod]) bool { return false },
	}
}

func isGFDPod(pod *corev1.Pod, gfdApp string) bool {
	if pod == nil || pod.Labels == nil {
		return false
	}
	return pod.Namespace == common.WorkloadsNamespace && pod.Labels["app"] == gfdApp
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
