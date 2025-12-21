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

package webhook

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestEnsurePoolTolerationBranches(t *testing.T) {
	poolKey := "gpu.deckhouse.io/pool-a"

	pod := &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "other", Operator: corev1.TolerationOpExists}}}}
	if err := ensurePoolToleration(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolToleration: %v", err)
	}

	pod = &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: poolKey, Operator: corev1.TolerationOpExists}}}}
	if err := ensurePoolToleration(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolToleration: %v", err)
	}

	pod = &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: poolKey}}}}
	if err := ensurePoolToleration(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolToleration: %v", err)
	}
	if got := pod.Spec.Tolerations[0]; got.Operator != corev1.TolerationOpEqual || got.Effect != corev1.TaintEffectNoSchedule || got.Value != "pool-a" {
		t.Fatalf("unexpected normalized toleration: %+v", got)
	}

	pod = &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: poolKey, Operator: corev1.TolerationOpEqual, Effect: corev1.TaintEffectNoSchedule, Value: "pool-a"}}}}
	if err := ensurePoolToleration(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolToleration: %v", err)
	}

	pod = &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: poolKey, Operator: corev1.TolerationOpEqual, Effect: corev1.TaintEffectNoExecute, Value: "pool-a"}}}}
	if err := ensurePoolToleration(pod, poolKey, "pool-a"); err == nil {
		t.Fatalf("expected unsupported effect error")
	}

	pod = &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: poolKey, Operator: "Weird"}}}}
	if err := ensurePoolToleration(pod, poolKey, "pool-a"); err == nil {
		t.Fatalf("expected unsupported operator error")
	}
}

func TestEnsurePoolAffinityBranches(t *testing.T) {
	poolKey := "gpu.deckhouse.io/pool-a"

	pod := &corev1.Pod{}
	if err := ensurePoolAffinity(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolAffinity: %v", err)
	}

	pod = &corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "other", Operator: corev1.NodeSelectorOpIn, Values: []string{"x"}}},
					}},
				},
			}},
		},
	}
	if err := ensurePoolAffinity(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolAffinity: %v", err)
	}

	pod = &corev1.Pod{
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{Key: poolKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"pool-a"}}},
					}},
				},
			}},
		},
	}
	if err := ensurePoolAffinity(pod, poolKey, "pool-a"); err != nil {
		t.Fatalf("ensurePoolAffinity: %v", err)
	}
}

func TestEnsureCustomTolerationsSkipsExistingKey(t *testing.T) {
	state := moduleconfig.DefaultState()
	state.Settings.Placement.CustomTolerationKeys = []string{"dedicated", "gpu-role"}
	store := moduleconfig.NewModuleConfigStore(state)

	pod := &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "dedicated", Operator: corev1.TolerationOpExists}}}}
	ensureCustomTolerations(pod, store)

	var foundDedicated, foundRole int
	for _, t := range pod.Spec.Tolerations {
		switch t.Key {
		case "dedicated":
			foundDedicated++
		case "gpu-role":
			foundRole++
		}
	}
	if foundDedicated != 1 || foundRole != 1 {
		t.Fatalf("expected existing toleration to be kept and new one added, got %+v", pod.Spec.Tolerations)
	}
}

func TestEnsureSpreadConstraintBranches(t *testing.T) {
	pod := &corev1.Pod{}
	if err := ensureSpreadConstraint(pod, "k", "v", ""); err != nil {
		t.Fatalf("ensureSpreadConstraint: %v", err)
	}

	poolKey := "gpu.deckhouse.io/pool-a"
	pod = &corev1.Pod{Spec: corev1.PodSpec{TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
		TopologyKey: "zone",
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{poolKey: "pool-a"},
		},
	}}}}

	if err := ensureSpreadConstraint(pod, poolKey, "pool-a", "zone"); err != nil {
		t.Fatalf("ensureSpreadConstraint: %v", err)
	}

	pod = &corev1.Pod{Spec: corev1.PodSpec{TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
		{
			TopologyKey: "other",
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{poolKey: "pool-a"},
			},
		},
		{
			TopologyKey: "zone",
		},
	}}}
	if err := ensureSpreadConstraint(pod, poolKey, "pool-a", "zone"); err != nil {
		t.Fatalf("ensureSpreadConstraint: %v", err)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 3 {
		t.Fatalf("expected constraint to be added, got %+v", pod.Spec.TopologySpreadConstraints)
	}
}
