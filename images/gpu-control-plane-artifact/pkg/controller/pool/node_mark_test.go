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
)

func TestNodeMarkAddsLabelWithoutTaint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().
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
	key := poolLabelKey(pool)
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

	key := poolLabelKey(&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}})
	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().
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
	key = poolLabelKey(pool)
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
	existingTaint := corev1.Taint{Key: poolLabelKey(pool), Value: "pool-a", Effect: corev1.TaintEffectNoSchedule}
	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{poolLabelKey(pool): "pool-a"}}, Spec: corev1.NodeSpec{Taints: []corev1.Taint{existingTaint}}},
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
	if hasTaint(node.Spec.Taints, poolLabelKey(pool), corev1.TaintEffectNoSchedule) {
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
