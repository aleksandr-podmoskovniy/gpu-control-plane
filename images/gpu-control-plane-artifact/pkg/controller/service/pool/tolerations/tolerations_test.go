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

package tolerations

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

type listErrorClient struct {
	client.Client
	err error
}

func (c listErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

func TestBuildCustomTolerationsSkipsEmptyAndDedupes(t *testing.T) {
	out := BuildCustom([]string{"", "a", "a", "b"})
	if len(out) != 2 {
		t.Fatalf("expected 2 tolerations, got %d: %+v", len(out), out)
	}
	if out[0].Operator != corev1.TolerationOpExists || out[1].Operator != corev1.TolerationOpExists {
		t.Fatalf("expected Exists operator, got %+v", out)
	}
}

func TestMergeTolerationsDedupesExtra(t *testing.T) {
	base := []corev1.Toleration{{Key: "k", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	extra := []corev1.Toleration{
		{Key: "k", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: "x", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
	}
	out := Merge(base, extra)
	if len(out) != 2 {
		t.Fatalf("expected 2 tolerations, got %d: %+v", len(out), out)
	}
}

func TestPoolNodeTolerationsNilClientAndListError(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}}

	if got := PoolNodeTolerations(context.Background(), nil, pool); got != nil {
		t.Fatalf("expected nil tolerations without client, got %+v", got)
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	errClient := listErrorClient{Client: base, err: errors.New("boom")}
	if got := PoolNodeTolerations(context.Background(), errClient, pool); got != nil {
		t.Fatalf("expected nil tolerations on list error, got %+v", got)
	}
}

func TestPoolNodeTolerationsCollectsAndDedupesTaints(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}}
	poolKey := poolcommon.PoolLabelKey(pool)

	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{poolKey: "alpha"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "a", Effect: corev1.TaintEffectNoSchedule},
				{Key: "a", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{poolKey: "alpha"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "a", Effect: corev1.TaintEffectNoSchedule},
				{Key: "b", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node1, node2).
		Build()

	got := PoolNodeTolerations(context.Background(), cl, pool)
	if len(got) != 2 {
		t.Fatalf("expected 2 tolerations, got %d: %+v", len(got), got)
	}
}
