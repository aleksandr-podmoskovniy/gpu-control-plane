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
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestToleratesTaint(t *testing.T) {
	taint := corev1.Taint{Key: "k", Value: "v", Effect: corev1.TaintEffectNoSchedule}

	cases := []struct {
		name       string
		toleration corev1.Toleration
		want       bool
	}{
		{name: "key mismatch", toleration: corev1.Toleration{Key: "other", Operator: corev1.TolerationOpExists}, want: false},
		{name: "effect mismatch", toleration: corev1.Toleration{Key: "k", Effect: corev1.TaintEffectNoExecute, Operator: corev1.TolerationOpExists}, want: false},
		{name: "exists tolerates", toleration: corev1.Toleration{Key: "k", Operator: corev1.TolerationOpExists}, want: true},
		{name: "empty operator tolerates", toleration: corev1.Toleration{Key: "k"}, want: true},
		{name: "equal empty value tolerates any", toleration: corev1.Toleration{Key: "k", Operator: corev1.TolerationOpEqual}, want: true},
		{name: "equal matching value tolerates", toleration: corev1.Toleration{Key: "k", Operator: corev1.TolerationOpEqual, Value: "v"}, want: true},
		{name: "equal different value does not", toleration: corev1.Toleration{Key: "k", Operator: corev1.TolerationOpEqual, Value: "x"}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toleratesTaint([]corev1.Toleration{tc.toleration}, taint)
			if got != tc.want {
				t.Fatalf("toleratesTaint()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnsureNodeTolerationsBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	poolKey := localPoolResourcePrefix + pool.Name

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{poolKey: pool.Name},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "k1", Value: "v1", Effect: corev1.TaintEffectNoSchedule},
				{Key: "k2", Value: "", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	d := NewPodDefaulter(testr.New(t), nil, cl)

	pod := &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "k1", Operator: corev1.TolerationOpExists}}}}
	if err := d.ensureNodeTolerations(context.Background(), pod, pool); err != nil {
		t.Fatalf("ensureNodeTolerations: %v", err)
	}
	if len(pod.Spec.Tolerations) != 2 {
		t.Fatalf("expected 2 tolerations, got %+v", pod.Spec.Tolerations)
	}
	if pod.Spec.Tolerations[1].Key != "k2" || pod.Spec.Tolerations[1].Operator != corev1.TolerationOpExists {
		t.Fatalf("expected Exists toleration for k2, got %+v", pod.Spec.Tolerations[1])
	}

	if err := d.ensureNodeTolerations(context.Background(), &corev1.Pod{}, nil); err != nil {
		t.Fatalf("expected noop for nil pool, got %v", err)
	}
}

func TestCollectPoolNodeTaintsPrefixAndListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: ""}}
	poolKey := clusterPoolResourcePrefix + pool.Name

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{poolKey: pool.Name},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "k", Value: "v", Effect: corev1.TaintEffectNoSchedule},
				{Key: "k", Value: "v", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}

	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	d := NewPodDefaulter(testr.New(t), nil, base)
	taints, err := d.collectPoolNodeTaints(context.Background(), pool)
	if err != nil {
		t.Fatalf("collectPoolNodeTaints: %v", err)
	}
	if len(taints) != 1 {
		t.Fatalf("expected deduped taints, got %+v", taints)
	}

	d = NewPodDefaulter(testr.New(t), nil, listErrorClient{Client: base, err: errors.New("boom")})
	if _, err := d.collectPoolNodeTaints(context.Background(), pool); err == nil {
		t.Fatalf("expected list error")
	}
}

func TestTopologyLabelPresentBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	d := NewPodDefaulter(testr.New(t), nil, base)

	ok, err := d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "")
	if err != nil || ok {
		t.Fatalf("expected (false,nil) for empty topologyKey, got (%v,%v)", ok, err)
	}

	d = NewPodDefaulter(testr.New(t), nil, listErrorClient{Client: base, err: errors.New("boom")})
	if _, err := d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "zone"); err == nil {
		t.Fatalf("expected list error")
	}

	d = NewPodDefaulter(testr.New(t), nil, base)
	ok, err = d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "zone")
	if err != nil || !ok {
		t.Fatalf("expected (true,nil) for no nodes yet, got (%v,%v)", ok, err)
	}

	poolKey := "gpu.deckhouse.io/pool-a"
	nodeWithLabel := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{poolKey: "pool-a", "zone": "a"}}}
	nodeWithoutLabel := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{poolKey: "pool-a"}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeWithLabel, nodeWithoutLabel).Build()
	d = NewPodDefaulter(testr.New(t), nil, cl)

	ok, err = d.topologyLabelPresent(context.Background(), poolKey, "pool-a", "zone")
	if err != nil || !ok {
		t.Fatalf("expected label to be detected, got (%v,%v)", ok, err)
	}

	cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeWithoutLabel).Build()
	d = NewPodDefaulter(testr.New(t), nil, cl)
	ok, err = d.topologyLabelPresent(context.Background(), poolKey, "pool-a", "zone")
	if err != nil || ok {
		t.Fatalf("expected missing label to return false, got (%v,%v)", ok, err)
	}
}
