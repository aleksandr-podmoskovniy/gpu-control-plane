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

package pool

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/testutil"
)

func TestNodeMarkHandlerNameAndClientRequirement(t *testing.T) {
	handler := NewNodeMarkHandler(testr.New(t), nil)
	if handler.Name() != "node-mark" {
		t.Fatalf("unexpected handler name: %s", handler.Name())
	}
	if _, err := handler.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}); err == nil {
		t.Fatalf("expected error when client is nil")
	}
}

func TestNodeMarkAddsLabelWithoutTaint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := testutil.WithNodeTaintIndexes(testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
			&v1alpha1.GPUDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
				Status: v1alpha1.GPUDeviceStatus{
					NodeName: "node1",
					PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a"},
				},
			},
		).
		Build()

	handler := NewNodeMarkHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	node := &corev1.Node{}
	if err := cl.Get(context.Background(), clientKey("node1"), node); err != nil {
		t.Fatalf("get node: %v", err)
	}
	key := PoolLabelKey(pool)
	if node.Labels[key] != "pool-a" {
		t.Fatalf("label not set")
	}
	if len(node.Spec.Taints) != 0 {
		t.Fatalf("expected no taints by default, got %+v", node.Spec.Taints)
	}
}

func TestNodeMarkRemovesLabelWhenEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	key := PoolLabelKey(&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}})
	cl := testutil.WithNodeTaintIndexes(testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{key: "pool-a"}}}).
		Build()

	handler := NewNodeMarkHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	node := &corev1.Node{}
	if err := cl.Get(context.Background(), clientKey("node1"), node); err != nil {
		t.Fatalf("get node: %v", err)
	}
	key = PoolLabelKey(pool)
	if _, ok := node.Labels[key]; ok {
		t.Fatalf("label should be removed")
	}
	if len(node.Spec.Taints) != 0 {
		t.Fatalf("expected no taints when devices gone, got %+v", node.Spec.Taints)
	}
}

func TestNodeMarkRemovesTaintWhenDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}
	existingTaint := corev1.Taint{Key: PoolLabelKey(pool), Value: "pool-a", Effect: corev1.TaintEffectNoSchedule}
	cl := testutil.WithNodeTaintIndexes(testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{PoolLabelKey(pool): "pool-a"}}, Spec: corev1.NodeSpec{Taints: []corev1.Taint{existingTaint}}},
			&v1alpha1.GPUDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
				Status: v1alpha1.GPUDeviceStatus{
					NodeName: "node1",
					PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a"},
				},
			},
		).
		Build()

	handler := NewNodeMarkHandler(testr.New(t), cl)
	taintsEnabled := false
	pool.Spec.Scheduling = v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: &taintsEnabled}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	node := &corev1.Node{}
	if err := cl.Get(context.Background(), clientKey("node1"), node); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if hasTaint(node.Spec.Taints, PoolLabelKey(pool), corev1.TaintEffectNoSchedule) {
		t.Fatalf("taint should be removed when taints disabled")
	}
}

func hasTaint(taints []corev1.Taint, key string, effect corev1.TaintEffect) bool {
	for _, t := range taints {
		if t.Key == key && t.Effect == effect {
			return true
		}
	}
	return false
}

func clientKey(name string) client.ObjectKey {
	return client.ObjectKey{Name: name}
}

func TestEnsureTaints(t *testing.T) {
	key := "gpu.deckhouse.io/pool-a"
	existing := []corev1.Taint{
		{Key: "other", Effect: corev1.TaintEffectNoSchedule},
		{Key: key, Effect: corev1.TaintEffectNoSchedule},
	}

	out, changed := ensureTaints(existing, nil, key)
	if !changed {
		t.Fatalf("expected taints to change when removing existing pool taint")
	}
	if hasTaint(out, key, corev1.TaintEffectNoSchedule) {
		t.Fatalf("expected pool taint removed")
	}

	_, changed = ensureTaints([]corev1.Taint{{Key: "other"}}, nil, key)
	if changed {
		t.Fatalf("expected no changes when pool taint is absent and desired is empty")
	}

	out, changed = ensureTaints(nil, []corev1.Taint{{Key: key, Effect: corev1.TaintEffectNoSchedule}}, key)
	if !changed || !hasTaint(out, key, corev1.TaintEffectNoSchedule) {
		t.Fatalf("expected desired taint to be present")
	}
}

func TestAlternatePoolLabelKey(t *testing.T) {
	if AlternatePoolLabelKey(nil) != "" {
		t.Fatalf("expected empty alternate pool label key for nil pool")
	}

	nsPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	if got := AlternatePoolLabelKey(nsPool); got != "cluster.gpu.deckhouse.io/pool-a" {
		t.Fatalf("unexpected alternate key: %s", got)
	}
}

func TestNodeMarkAddsTaintAndRemovesAlternatePrefix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	poolKey := PoolLabelKey(pool)
	altKey := AlternatePoolLabelKey(pool)
	value := "pool-a"

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{altKey: value},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: altKey, Value: value, Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns"},
		},
	}

	cl := testutil.WithNodeTaintIndexes(testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(node, device).
		Build()

	handler := NewNodeMarkHandler(testr.New(t), cl)
	taintsEnabled := true
	pool.Spec.Scheduling = v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: &taintsEnabled}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	updated := &corev1.Node{}
	if err := cl.Get(context.Background(), clientKey("node1"), updated); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated.Labels[poolKey] != value {
		t.Fatalf("expected pool label %q=%q, got %+v", poolKey, value, updated.Labels)
	}
	if _, ok := updated.Labels[altKey]; ok {
		t.Fatalf("expected alternate label %q to be removed", altKey)
	}
	if !hasTaint(updated.Spec.Taints, poolKey, corev1.TaintEffectNoSchedule) {
		t.Fatalf("expected pool taint to be present")
	}
	if hasTaint(updated.Spec.Taints, altKey, corev1.TaintEffectNoSchedule) {
		t.Fatalf("expected alternate taint to be removed")
	}
}

func TestNodeMarkSyncNodeNoChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := testutil.WithNodeTaintIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithObjects(node).
		Build()

	handler := NewNodeMarkHandler(testr.New(t), cl)
	if err := handler.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool-a", "cluster.gpu.deckhouse.io/pool-a", false, false); err != nil {
		t.Fatalf("syncNode: %v", err)
	}
}
