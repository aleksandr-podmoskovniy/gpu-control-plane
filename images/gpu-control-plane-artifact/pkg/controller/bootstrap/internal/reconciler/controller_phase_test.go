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

package reconciler

import (
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestEffectiveBootstrapPhaseDefault(t *testing.T) {
	inventory := &v1alpha1.GPUNodeState{}
	if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
		t.Fatalf("expected default phase Validating, got %s", phase)
	}
}

func TestEffectiveBootstrapPhaseVariants(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionReadyForPooling, Status: metav1.ConditionTrue})
		if phase := effectiveBootstrapPhase(inventory); phase != "Ready" {
			t.Fatalf("expected phase Ready, got %s", phase)
		}
	})

	t.Run("validating-driver-not-ready", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionToolkitReady, Status: metav1.ConditionTrue})
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionDriverReady, Status: metav1.ConditionFalse})
		if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
			t.Fatalf("expected phase Validating, got %s", phase)
		}
	})

	t.Run("validating-toolkit-not-ready", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionDriverReady, Status: metav1.ConditionTrue})
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionToolkitReady, Status: metav1.ConditionFalse})
		if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
			t.Fatalf("expected phase Validating, got %s", phase)
		}
	})

	t.Run("monitoring-not-ready", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionDriverReady, Status: metav1.ConditionTrue})
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionToolkitReady, Status: metav1.ConditionTrue})
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionMonitoringReady, Status: metav1.ConditionFalse})
		if phase := effectiveBootstrapPhase(inventory); phase != "Monitoring" {
			t.Fatalf("expected phase Monitoring, got %s", phase)
		}
	})

	t.Run("infra-ready-but-not-ready-for-pooling", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionDriverReady, Status: metav1.ConditionTrue})
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionToolkitReady, Status: metav1.ConditionTrue})
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionMonitoringReady, Status: metav1.ConditionTrue})
		if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
			t.Fatalf("expected phase Validating, got %s", phase)
		}
	})
}
