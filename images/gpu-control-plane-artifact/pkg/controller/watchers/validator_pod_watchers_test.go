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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	commonpod "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/pod"
)

func TestPodReady(t *testing.T) {
	if commonpod.IsReady(nil) {
		t.Fatalf("expected nil pod to be not ready")
	}

	pod := &corev1.Pod{}
	if commonpod.IsReady(pod) {
		t.Fatalf("expected empty pod to be not ready")
	}

	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	if commonpod.IsReady(pod) {
		t.Fatalf("expected pod with Ready=false to be not ready")
	}

	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if !commonpod.IsReady(pod) {
		t.Fatalf("expected pod with Ready=true to be ready")
	}
}

func TestValidatorPodPredicates(t *testing.T) {
	p := validatorPodPredicates()

	if p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: nil}) {
		t.Fatalf("expected create predicate to ignore nil pod")
	}
	if p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: &corev1.Pod{}}) {
		t.Fatalf("expected create predicate to ignore non-validator pods")
	}
	if !p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nvidia-operator-validator", "pool": "pool-a"}}}}) {
		t.Fatalf("expected create predicate to accept validator pool pods")
	}

	oldPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nvidia-operator-validator", "pool": "pool-a"}}, Spec: corev1.PodSpec{NodeName: "node-a"}}
	newPod := oldPod.DeepCopy()
	newPod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	oldPod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected update predicate to trigger when readiness changes")
	}

	newPod = oldPod.DeepCopy()
	newPod.Spec.NodeName = "node-b"
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatalf("expected update predicate to trigger when nodeName changes")
	}

	nonValidator := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "other"}}}
	if p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: nonValidator}) {
		t.Fatalf("expected update predicate to ignore updates for non-validator pods")
	}
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: nonValidator, ObjectNew: oldPod}) {
		t.Fatalf("expected update predicate to trigger when a validator pod appears")
	}

	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: nil, ObjectNew: oldPod}) {
		t.Fatalf("expected update predicate to pass through nil old")
	}
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldPod, ObjectNew: nil}) {
		t.Fatalf("expected update predicate to pass through nil new")
	}

	if !p.Delete(event.TypedDeleteEvent[*corev1.Pod]{Object: oldPod}) {
		t.Fatalf("expected delete predicate to trigger for validator pods")
	}
	if p.Delete(event.TypedDeleteEvent[*corev1.Pod]{Object: nonValidator}) {
		t.Fatalf("expected delete predicate to ignore non-validator pods")
	}
	if p.Generic(event.TypedGenericEvent[*corev1.Pod]{Object: oldPod}) {
		t.Fatalf("expected generic predicate to be ignored")
	}
}
