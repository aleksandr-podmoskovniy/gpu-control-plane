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

package bootstrap

import (
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	bootmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/bootstrap"
)

func TestUpdateBootstrapMetricsSetsPhaseAndConditions(t *testing.T) {
	nodeName := "node-metrics-update"
	rec := &Reconciler{}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: nodeName},
	}
	apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionReadyForPooling, Status: metav1.ConditionTrue})
	rec.updateBootstrapMetrics(nodeName, "Validating", inventory)

	if v, ok := gaugeValue(t, bootmetrics.BootstrapNodePhaseMetric, map[string]string{"node": nodeName, "phase": "Ready"}); !ok || v != 1 {
		t.Fatalf("expected phase gauge to be set, got %f", v)
	}
	if v, ok := gaugeValue(t, bootmetrics.BootstrapConditionMetric, map[string]string{"node": nodeName, "condition": conditionReadyForPooling}); !ok || v != 1 {
		t.Fatalf("expected condition gauge to be set, got %f", v)
	}
}

func TestClearBootstrapMetricsRemovesValues(t *testing.T) {
	nodeName := "node-metrics-clear"
	rec := &Reconciler{}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: nodeName},
	}
	apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionReadyForPooling, Status: metav1.ConditionTrue})
	rec.updateBootstrapMetrics(nodeName, "Validating", inventory)

	if _, ok := findMetric(t, bootmetrics.BootstrapNodePhaseMetric, map[string]string{"node": nodeName, "phase": "Ready"}); !ok {
		t.Fatalf("expected phase gauge populated")
	}
	if _, ok := findMetric(t, bootmetrics.BootstrapConditionMetric, map[string]string{"node": nodeName, "condition": conditionReadyForPooling}); !ok {
		t.Fatalf("expected condition gauge populated")
	}

	rec.clearBootstrapMetrics(nodeName)

	if _, ok := findMetric(t, bootmetrics.BootstrapNodePhaseMetric, map[string]string{"node": nodeName, "phase": "Ready"}); ok {
		t.Fatalf("expected phase gauge cleared")
	}
	if _, ok := findMetric(t, bootmetrics.BootstrapConditionMetric, map[string]string{"node": nodeName, "condition": conditionReadyForPooling}); ok {
		t.Fatalf("expected condition gauge cleared")
	}
}
