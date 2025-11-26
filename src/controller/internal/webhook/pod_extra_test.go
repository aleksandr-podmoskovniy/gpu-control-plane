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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestHasToleration(t *testing.T) {
	if hasToleration(nil, "key") {
		t.Fatalf("expected false for nil tolerations")
	}
	tols := []corev1.Toleration{{Key: "a"}, {Key: "b"}}
	if !hasToleration(tols, "b") {
		t.Fatalf("expected true when key present")
	}
}

func TestGVK(t *testing.T) {
	m := &podMutator{}
	gvk := m.GVK()
	if gvk.Kind != "Pod" || gvk.Version != "v1" {
		t.Fatalf("unexpected GVK: %v", gvk)
	}
}

func TestEnsureSpreadConstraintConflicts(t *testing.T) {
	pod := &corev1.Pod{}
	// conflict on existing value
	pod.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		TopologyKey:   "topo",
		LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "other"}},
	}}
	if err := ensureSpreadConstraint(pod, "gpu", "pool", "topo"); err == nil {
		t.Fatalf("expected conflict error")
	}
	// success add new constraint
	pod.Spec.TopologySpreadConstraints = nil
	if err := ensureSpreadConstraint(pod, "gpu", "pool", "topo"); err != nil {
		t.Fatalf("expected constraint added, got %v", err)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected 1 spread constraint, got %d", len(pod.Spec.TopologySpreadConstraints))
	}
	// existing with same value should return nil
	if err := ensureSpreadConstraint(pod, "gpu", "pool", "topo"); err != nil {
		t.Fatalf("expected no error when constraint already matches, got %v", err)
	}
	// skip when topologyKey empty
	if err := ensureSpreadConstraint(pod, "gpu", "pool", ""); err != nil {
		t.Fatalf("expected nil when topology empty, got %v", err)
	}

	// selector missing should be ignored and new constraint added
	pod.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		TopologyKey: "topo",
	}}
	if err := ensureSpreadConstraint(pod, "gpu", "pool", "topo"); err != nil {
		t.Fatalf("expected addition when selector missing, got %v", err)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 2 {
		t.Fatalf("expected second constraint appended")
	}
}

func TestCollectPoolsCoversInitContainers(t *testing.T) {
	qty := resource.MustParse("1")
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/a"): qty}},
		}},
		InitContainers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/b"): qty}},
		}},
	}}
	pools := collectPools(pod)
	if len(pools) != 2 {
		t.Fatalf("expected two pools, got %d", len(pools))
	}
	// pod without gpu resources
	empty := collectPools(&corev1.Pod{})
	if len(empty) != 0 {
		t.Fatalf("expected empty set, got %v", empty)
	}

	// skip empty pool name
	pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/"): qty}
	if pools := collectPools(pod); len(pools) != 1 { // only from init container request
		t.Fatalf("expected one pool after skipping empty name, got %d", len(pools))
	}
}

func TestPoolSchedulingAndTaintsEnabled(t *testing.T) {
	store := config.NewModuleConfigStore(moduleconfig.State{
		Settings: moduleconfig.Settings{
			Scheduling: moduleconfig.SchedulingSettings{DefaultStrategy: "Spread", TopologyKey: "topo"},
		},
	})
	m := &podMutator{store: store}
	strategy, topo := m.poolScheduling(nil)
	if strategy != "Spread" || topo != "topo" {
		t.Fatalf("expected scheduling from store, got %s/%s", strategy, topo)
	}

	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: ptr.To(false)}}}
	if m.poolTaintsEnabled(pool) {
		t.Fatalf("expected taints disabled")
	}
}

func TestEnsurePoolTolerationAndAffinity(t *testing.T) {
	pod := &corev1.Pod{}
	poolKey := "gpu.deckhouse.io/pool"

	// adds toleration and affinity when missing
	if err := ensurePoolToleration(pod, poolKey, "pool"); err != nil {
		t.Fatalf("toleration add failed: %v", err)
	}
	if err := ensurePoolAffinity(pod, poolKey, "pool"); err != nil {
		t.Fatalf("affinity add failed: %v", err)
	}

	// conflict toleration on a fresh pod
	conflictPod := &corev1.Pod{Spec: corev1.PodSpec{
		Tolerations: []corev1.Toleration{{Key: poolKey, Value: "other", Effect: corev1.TaintEffectNoSchedule}},
	}}
	if err := ensurePoolToleration(conflictPod, poolKey, "pool"); err == nil {
		t.Fatalf("expected conflict on different toleration value")
	}

	// existing matching toleration should return nil
	pod.Spec.Tolerations = []corev1.Toleration{{Key: poolKey, Value: "pool", Effect: corev1.TaintEffectNoSchedule}}
	if err := ensurePoolToleration(pod, poolKey, "pool"); err != nil {
		t.Fatalf("expected no error for matching toleration")
	}

	// conflict affinity when different value present
	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values = []string{"other"}
	if err := ensurePoolAffinity(pod, poolKey, "pool"); err == nil {
		t.Fatalf("expected conflict on affinity mismatch")
	}

	// add to existing term without pool expression
	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "other", Operator: corev1.NodeSelectorOpExists}}}},
	}
	if err := ensurePoolAffinity(pod, poolKey, "pool"); err != nil {
		t.Fatalf("expected pool expression appended, got %v", err)
	}

	// existing matching expression should be accepted
	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: poolKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"pool"}}}}},
	}
	if err := ensurePoolAffinity(pod, poolKey, "pool"); err != nil {
		t.Fatalf("expected matching affinity to be accepted, got %v", err)
	}
}

func TestEnsureCustomTolerations(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "keep"}}}}
	ensureCustomTolerations(pod, nil) // should no-op
	if len(pod.Spec.Tolerations) != 1 {
		t.Fatalf("expected tolerations untouched when store nil")
	}

	state := moduleconfig.State{Settings: moduleconfig.Settings{Placement: moduleconfig.PlacementSettings{CustomTolerationKeys: []string{"keep", "new"}}}}
	store := config.NewModuleConfigStore(state)
	ensureCustomTolerations(pod, store)
	if len(pod.Spec.Tolerations) != 2 {
		t.Fatalf("expected new toleration appended once, got %d", len(pod.Spec.Tolerations))
	}
}

func TestEnsureSpreadConstraintOtherTopology(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		TopologyKey:   "other",
		LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
	}}
	if err := ensureSpreadConstraint(pod, "gpu", "pool", "topo"); err != nil {
		t.Fatalf("expected constraint appended when topology differs, got %v", err)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 2 {
		t.Fatalf("expected second constraint added, got %d", len(pod.Spec.TopologySpreadConstraints))
	}
}

func TestCollectPoolsRequests(t *testing.T) {
	qty := resource.MustParse("1")
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("gpu.deckhouse.io/x"): qty,
					corev1.ResourceCPU:                        qty,
				},
			},
		}},
	}}
	if pools := collectPools(pod); len(pools) != 1 {
		t.Fatalf("expected one pool from requests, got %d", len(pools))
	}
}
