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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
)

func TestGPUPoolPredicates(t *testing.T) {
	p := GPUPoolPredicates()

	if !p.Create(event.TypedCreateEvent[*v1alpha1.GPUPool]{Object: &v1alpha1.GPUPool{}}) {
		t.Fatalf("expected create to pass")
	}

	oldPool := &v1alpha1.GPUPool{}
	newPool := &v1alpha1.GPUPool{}
	if p.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: oldPool, ObjectNew: newPool}) {
		t.Fatalf("expected update to be filtered when no changes")
	}

	newPool = oldPool.DeepCopy()
	newPool.Generation++
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: oldPool, ObjectNew: newPool}) {
		t.Fatalf("expected update to pass when generation changes")
	}

	newPool = oldPool.DeepCopy()
	newPool.Status.Capacity.Total = 1
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: oldPool, ObjectNew: newPool}) {
		t.Fatalf("expected update to pass when total changes")
	}

	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: nil, ObjectNew: oldPool}) {
		t.Fatalf("expected update to pass when old is nil")
	}
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: oldPool, ObjectNew: nil}) {
		t.Fatalf("expected update to pass when new is nil")
	}

	if !p.Delete(event.TypedDeleteEvent[*v1alpha1.GPUPool]{Object: &v1alpha1.GPUPool{}}) {
		t.Fatalf("expected delete to pass")
	}
	if p.Generic(event.TypedGenericEvent[*v1alpha1.GPUPool]{Object: &v1alpha1.GPUPool{}}) {
		t.Fatalf("expected generic to be filtered")
	}
}

func TestClusterGPUPoolPredicates(t *testing.T) {
	p := ClusterGPUPoolPredicates()

	if !p.Create(event.TypedCreateEvent[*v1alpha1.ClusterGPUPool]{Object: &v1alpha1.ClusterGPUPool{}}) {
		t.Fatalf("expected create to pass")
	}

	oldPool := &v1alpha1.ClusterGPUPool{}
	newPool := &v1alpha1.ClusterGPUPool{}
	if p.Update(event.TypedUpdateEvent[*v1alpha1.ClusterGPUPool]{ObjectOld: oldPool, ObjectNew: newPool}) {
		t.Fatalf("expected update to be filtered when no changes")
	}

	newPool = oldPool.DeepCopy()
	newPool.Generation++
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.ClusterGPUPool]{ObjectOld: oldPool, ObjectNew: newPool}) {
		t.Fatalf("expected update to pass when generation changes")
	}

	newPool = oldPool.DeepCopy()
	newPool.Status.Capacity.Total = 1
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.ClusterGPUPool]{ObjectOld: oldPool, ObjectNew: newPool}) {
		t.Fatalf("expected update to pass when total changes")
	}

	if !p.Update(event.TypedUpdateEvent[*v1alpha1.ClusterGPUPool]{ObjectOld: nil, ObjectNew: oldPool}) {
		t.Fatalf("expected update to pass when old is nil")
	}
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.ClusterGPUPool]{ObjectOld: oldPool, ObjectNew: nil}) {
		t.Fatalf("expected update to pass when new is nil")
	}

	if !p.Delete(event.TypedDeleteEvent[*v1alpha1.ClusterGPUPool]{Object: &v1alpha1.ClusterGPUPool{}}) {
		t.Fatalf("expected delete to pass")
	}
	if p.Generic(event.TypedGenericEvent[*v1alpha1.ClusterGPUPool]{Object: &v1alpha1.ClusterGPUPool{}}) {
		t.Fatalf("expected generic to be filtered")
	}
}

func TestGPUWorkloadPodPredicates(t *testing.T) {
	p := GPUWorkloadPodPredicates(podlabels.PoolScopeNamespaced)

	if p.Generic(event.TypedGenericEvent[*corev1.Pod]{Object: &corev1.Pod{}}) {
		t.Fatalf("expected generic to be filtered")
	}

	matching := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		podlabels.PoolNameKey:  "pool-a",
		podlabels.PoolScopeKey: podlabels.PoolScopeNamespaced,
	}}}
	if !p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: matching}) {
		t.Fatalf("expected create to pass for matching pod")
	}
	if !p.Delete(event.TypedDeleteEvent[*corev1.Pod]{Object: matching}) {
		t.Fatalf("expected delete to pass for matching pod")
	}

	notMatching := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		podlabels.PoolNameKey:  "pool-a",
		podlabels.PoolScopeKey: podlabels.PoolScopeCluster,
	}}}
	if p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: notMatching}) {
		t.Fatalf("expected create to be filtered for wrong scope")
	}

	if p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: matching, ObjectNew: notMatching}) {
		t.Fatalf("expected update to be filtered when new pod is not a matching workload pod")
	}

	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: notMatching, ObjectNew: matching}) {
		t.Fatalf("expected update to pass when pod starts matching")
	}

	oldPod := matching.DeepCopy()
	newPod := matching.DeepCopy()
	newPod.Spec.NodeName = "node1"
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected update to pass when node name changes")
	}

	oldPod = matching.DeepCopy()
	newPod = matching.DeepCopy()
	newPod.Status.Phase = corev1.PodRunning
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected update to pass when phase changes")
	}

	oldPod = matching.DeepCopy()
	newPod = matching.DeepCopy()
	now := metav1.Now()
	newPod.DeletionTimestamp = &now
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected update to pass when deletion timestamp changes")
	}

	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: nil, ObjectNew: matching}) {
		t.Fatalf("expected update to pass when old is nil")
	}
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: matching, ObjectNew: nil}) {
		t.Fatalf("expected update to pass when new is nil")
	}

	oldPod = matching.DeepCopy()
	oldPod.Spec.NodeName = "same"
	oldPod.Status.Phase = corev1.PodRunning
	newPod = oldPod.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected update to be filtered when nothing changed")
	}
}

func TestIsGPUWorkloadPod(t *testing.T) {
	if isGPUWorkloadPod(nil, podlabels.PoolScopeNamespaced) {
		t.Fatalf("expected nil pod to be false")
	}
	if isGPUWorkloadPod(&corev1.Pod{}, podlabels.PoolScopeNamespaced) {
		t.Fatalf("expected nil labels to be false")
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		podlabels.PoolNameKey:  "  ",
		podlabels.PoolScopeKey: podlabels.PoolScopeNamespaced,
	}}}
	if isGPUWorkloadPod(pod, podlabels.PoolScopeNamespaced) {
		t.Fatalf("expected empty pool name to be false")
	}

	pod = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		podlabels.PoolNameKey:  "pool-a",
		podlabels.PoolScopeKey: podlabels.PoolScopeNamespaced,
	}}}
	if !isGPUWorkloadPod(pod, podlabels.PoolScopeNamespaced) {
		t.Fatalf("expected matching scope pod to be true")
	}
	if isGPUWorkloadPod(pod, podlabels.PoolScopeCluster) {
		t.Fatalf("expected different scope pod to be false")
	}
}
