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

package service

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestDetectGPUPort(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "gfd-extender",
					Ports: []corev1.ContainerPort{{ContainerPort: 1234}},
				},
			},
		},
	}
	if port := detectGPUPort(pod); port != 1234 {
		t.Fatalf("expected gfd-extender port from container, got %d", port)
	}
	pod.Spec.Containers[0].Name = "other"
	if port := detectGPUPort(pod); port != 0 {
		t.Fatalf("expected zero port when container missing, got %d", port)
	}
}

func TestIsPodReadyFalseCases(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}
	if isPodReady(pod) {
		t.Fatalf("pending pod should not be ready")
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = "1.2.3.4"
	if isPodReady(pod) {
		t.Fatalf("pod without ready condition should not be ready")
	}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	if isPodReady(pod) {
		t.Fatalf("pod with ready=false should not be ready")
	}
}
