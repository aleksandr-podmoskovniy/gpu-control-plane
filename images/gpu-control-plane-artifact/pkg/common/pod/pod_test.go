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

package pod

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestIsReady(t *testing.T) {
	if IsReady(nil) {
		t.Fatalf("expected nil pod to be not ready")
	}
	if IsReady(&corev1.Pod{}) {
		t.Fatalf("expected pod without conditions to be not ready")
	}
	if IsReady(&corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}}}) {
		t.Fatalf("expected PodReady false to be not ready")
	}
	if !IsReady(&corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}) {
		t.Fatalf("expected PodReady true to be ready")
	}
}

