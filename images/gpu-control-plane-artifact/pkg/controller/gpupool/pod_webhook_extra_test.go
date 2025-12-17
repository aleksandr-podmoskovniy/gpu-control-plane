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

package gpupool

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
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
	store := moduleconfig.NewModuleConfigStore(moduleconfig.State{
		Settings: moduleconfig.Settings{
			Scheduling: moduleconfig.SchedulingSettings{DefaultStrategy: "Spread", TopologyKey: "topo"},
		},
	})
	d := &PodDefaulter{store: store}
	strategy, topo := d.poolScheduling(nil)
	if strategy != "Spread" || topo != "topo" {
		t.Fatalf("expected scheduling from store, got %s/%s", strategy, topo)
	}

	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: ptr.To(false)}}}
	if d.poolTaintsEnabled(pool) {
		t.Fatalf("expected taints disabled")
	}
}

func TestEnsurePoolTolerationAndAffinity(t *testing.T) {
	poolKey := "gpu.deckhouse.io/pool"

	// adds toleration and affinity when missing
	pod := &corev1.Pod{}
	if err := ensurePoolToleration(pod, poolKey, "pool"); err != nil {
		t.Fatalf("toleration add failed: %v", err)
	}
	if err := ensurePoolAffinity(pod, poolKey, "pool"); err != nil {
		t.Fatalf("affinity add failed: %v", err)
	}

	// normalizes empty operator/effect/value
	normalizePod := &corev1.Pod{Spec: corev1.PodSpec{
		Tolerations: []corev1.Toleration{{Key: poolKey}},
	}}
	if err := ensurePoolToleration(normalizePod, poolKey, "pool"); err != nil {
		t.Fatalf("expected normalization to succeed, got %v", err)
	}
	if normalizePod.Spec.Tolerations[0].Operator != corev1.TolerationOpEqual || normalizePod.Spec.Tolerations[0].Effect != corev1.TaintEffectNoSchedule || normalizePod.Spec.Tolerations[0].Value != "pool" {
		t.Fatalf("normalization failed: %+v", normalizePod.Spec.Tolerations[0])
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

	// existing empty value should be normalized
	pod.Spec.Tolerations = []corev1.Toleration{{Key: poolKey, Value: "", Effect: corev1.TaintEffectNoSchedule}}
	if err := ensurePoolToleration(pod, poolKey, "pool"); err != nil {
		t.Fatalf("expected normalization, got %v", err)
	}
	if pod.Spec.Tolerations[0].Value != "pool" {
		t.Fatalf("toleration value not normalized: %q", pod.Spec.Tolerations[0].Value)
	}

	// existing Exists operator is compatible
	pod.Spec.Tolerations = []corev1.Toleration{{Key: poolKey, Operator: corev1.TolerationOpExists}}
	if err := ensurePoolToleration(pod, poolKey, "pool"); err != nil {
		t.Fatalf("expected Exists toleration to be accepted, got %v", err)
	}

	// toleration with other key should be ignored and new one added
	pod.Spec.Tolerations = []corev1.Toleration{{Key: "other", Operator: corev1.TolerationOpExists}}
	if err := ensurePoolToleration(pod, poolKey, "pool"); err != nil {
		t.Fatalf("expected new toleration to be added when other keys present, got %v", err)
	}
	if len(pod.Spec.Tolerations) < 2 {
		t.Fatalf("expected appended toleration when other keys present")
	}

	// unsupported effect should be rejected
	pod.Spec.Tolerations = []corev1.Toleration{{Key: poolKey, Value: "pool", Effect: corev1.TaintEffectPreferNoSchedule}}
	if err := ensurePoolToleration(pod, poolKey, "pool"); err == nil {
		t.Fatalf("expected unsupported effect error")
	}

	// unsupported operator should be rejected
	pod.Spec.Tolerations = []corev1.Toleration{{Key: poolKey, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	pod.Spec.Tolerations[0].Operator = "DoesNotExist"
	if err := ensurePoolToleration(pod, poolKey, "pool"); err == nil {
		t.Fatalf("expected unsupported operator error")
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
	store := moduleconfig.NewModuleConfigStore(state)
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

func TestEnsureNodeTolerations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "gpu-node-1",
			Labels: map[string]string{"gpu.deckhouse.io/pool-a": "pool-a"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "dedicated.apiac.ru", Value: "w-gpu", Effect: corev1.TaintEffectNoSchedule},
				{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	d := &PodDefaulter{client: cl}
	pod := &corev1.Pod{}

	if err := d.ensureNodeTolerations(context.Background(), pod, pool); err != nil {
		t.Fatalf("ensureNodeTolerations returned error: %v", err)
	}
	if len(pod.Spec.Tolerations) != 2 {
		t.Fatalf("expected two tolerations from node taints, got %d", len(pod.Spec.Tolerations))
	}
	if !toleratesTaint(pod.Spec.Tolerations, node.Spec.Taints[0]) || !toleratesTaint(pod.Spec.Tolerations, node.Spec.Taints[1]) {
		t.Fatalf("tolerations do not match node taints: %+v", pod.Spec.Tolerations)
	}

	// existing matching toleration should prevent duplicates and still allow other taints to be added
	pod.Spec.Tolerations = []corev1.Toleration{{Key: "dedicated.apiac.ru", Operator: corev1.TolerationOpExists}}
	if err := d.ensureNodeTolerations(context.Background(), pod, pool); err != nil {
		t.Fatalf("ensureNodeTolerations returned error: %v", err)
	}
	if count := len(pod.Spec.Tolerations); count != 2 {
		t.Fatalf("expected deduped tolerations, got %d", count)
	}

	// pool or client missing should no-op
	d = &PodDefaulter{}
	pod = &corev1.Pod{}
	if err := d.ensureNodeTolerations(context.Background(), pod, nil); err != nil {
		t.Fatalf("expected nil error without pool/client, got %v", err)
	}

	// missing node in API should be tolerated (log only)
	d = &PodDefaulter{client: cl}
	pod = &corev1.Pod{}
	missingPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "absent", Namespace: "gpu-team"}}
	if err := d.ensureNodeTolerations(context.Background(), pod, missingPool); err != nil {
		t.Fatalf("expected no error when pool nodes are not labelled yet, got %v", err)
	}
	if len(pod.Spec.Tolerations) != 0 {
		t.Fatalf("expected no tolerations when nodes not found, got %v", pod.Spec.Tolerations)
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

	// cluster pool prefix is supported
	pod = &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceName("cluster.gpu.deckhouse.io/z"): qty},
			},
		}},
	}}
	if pools := collectPools(pod); len(pools) != 1 {
		t.Fatalf("expected one pool from cluster prefix, got %d", len(pools))
	}
}

func TestToleratesTaintVariants(t *testing.T) {
	taint := corev1.Taint{Key: "k", Value: "v", Effect: corev1.TaintEffectNoSchedule}

	// Exists toleration with empty effect tolerates any value/effect
	tols := []corev1.Toleration{{Key: "k", Operator: corev1.TolerationOpExists}}
	if !toleratesTaint(tols, taint) {
		t.Fatalf("expected exists toleration to match")
	}

	// Effect mismatch should not tolerate
	tols = []corev1.Toleration{{Key: "k", Operator: corev1.TolerationOpEqual, Value: "v", Effect: corev1.TaintEffectPreferNoSchedule}}
	if toleratesTaint(tols, taint) {
		t.Fatalf("expected effect mismatch to fail")
	}

	// Equal operator with different value should not tolerate
	tols = []corev1.Toleration{{Key: "k", Operator: corev1.TolerationOpEqual, Value: "other", Effect: corev1.TaintEffectNoSchedule}}
	if toleratesTaint(tols, taint) {
		t.Fatalf("expected value mismatch to fail")
	}

	// Empty operator should tolerate (treated as Exists)
	tols = []corev1.Toleration{{Key: "k"}}
	if !toleratesTaint(tols, taint) {
		t.Fatalf("expected empty operator to tolerate")
	}

	// Empty value with Equal should tolerate any taint value
	tols = []corev1.Toleration{{Key: "k", Operator: corev1.TolerationOpEqual, Value: "", Effect: corev1.TaintEffectNoSchedule}}
	if !toleratesTaint(tols, taint) {
		t.Fatalf("expected empty value to tolerate")
	}
}

func TestMutatorResolvePoolNamespaceEmpty(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	if _, err := resolvePoolByRequest(context.Background(), cl, poolRequest{name: "pool", keyPrefix: localPoolResourcePrefix}, ""); err == nil {
		t.Fatalf("expected error when namespace empty")
	}
}
