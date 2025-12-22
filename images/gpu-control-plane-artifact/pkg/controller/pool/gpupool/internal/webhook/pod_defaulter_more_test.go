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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

func TestPodDefaulterDefaultErrorPaths(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName(localPoolResourcePrefix + "pool-a"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}

	d := NewPodDefaulter(testr.New(t), nil, nil)
	if err := d.Default(context.Background(), &corev1.Namespace{}); err == nil {
		t.Fatalf("expected type error")
	}

	{
		conflict := pod.DeepCopy()
		conflict.Labels = map[string]string{poolcommon.PoolNameKey: "other"}
		if err := d.Default(context.Background(), conflict); err == nil {
			t.Fatalf("expected pool usage labels conflict error")
		}
	}

	{
		conflict := pod.DeepCopy()
		conflict.Spec.NodeSelector = map[string]string{localPoolResourcePrefix + "pool-a": "other"}
		if err := d.Default(context.Background(), conflict); err == nil {
			t.Fatalf("expected nodeSelector conflict error")
		}
	}

	{
		conflict := pod.DeepCopy()
		conflict.Spec.Tolerations = []corev1.Toleration{{Key: localPoolResourcePrefix + "pool-a", Operator: "Weird"}}
		if err := d.Default(context.Background(), conflict); err == nil {
			t.Fatalf("expected pool toleration error")
		}
	}

	{
		conflict := pod.DeepCopy()
		conflict.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      localPoolResourcePrefix + "pool-a",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"other"},
					}},
				}},
			},
		}}
		if err := d.Default(context.Background(), conflict); err == nil {
			t.Fatalf("expected pool affinity error")
		}
	}

	{
		state := moduleconfig.DefaultState()
		state.Settings.Scheduling.DefaultStrategy = string(v1alpha1.GPUPoolSchedulingSpread)
		state.Settings.Scheduling.TopologyKey = "zone"
		store := moduleconfig.NewModuleConfigStore(state)
		d = NewPodDefaulter(testr.New(t), store, nil)

		withConflictConstraint := pod.DeepCopy()
		withConflictConstraint.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
			TopologyKey: "zone",
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{localPoolResourcePrefix + "pool-a": "other"},
			},
		}}
		if err := d.Default(context.Background(), withConflictConstraint); err == nil {
			t.Fatalf("expected topology spread constraint conflict")
		}

		noConflict := pod.DeepCopy()
		if err := d.Default(context.Background(), noConflict); err != nil {
			t.Fatalf("expected topology spread to be added, got %v", err)
		}
	}
}

func TestPodDefaulterDefaultClientErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("ns")
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}

	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName(localPoolResourcePrefix + "pool-a"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}

	d := NewPodDefaulter(testr.New(t), nil, listErrorClient{Client: base, err: errors.New("list error")})
	if err := d.Default(context.Background(), pod.DeepCopy()); err == nil {
		t.Fatalf("expected node tolerations list error")
	}

	taintsEnabled := false
	pool.Spec.Scheduling.TaintsEnabled = ptr.To(taintsEnabled)
	pool.Spec.Scheduling.Strategy = v1alpha1.GPUPoolSchedulingSpread
	pool.Spec.Scheduling.TopologyKey = "zone"
	if err := base.Update(context.Background(), pool); err != nil {
		t.Fatalf("update pool: %v", err)
	}

	d = NewPodDefaulter(testr.New(t), nil, listErrorClient{Client: base, err: errors.New("list error")})
	if err := d.Default(context.Background(), pod.DeepCopy()); err == nil {
		t.Fatalf("expected topologyLabelPresent list error")
	}

	clusterPod := pod.DeepCopy()
	clusterPod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		corev1.ResourceName(clusterPoolResourcePrefix + "missing"): *resource.NewQuantity(1, resource.DecimalSI),
	}
	if err := d.Default(context.Background(), clusterPod); err == nil {
		t.Fatalf("expected resolvePoolByRequest error for missing ClusterGPUPool")
	}
}
