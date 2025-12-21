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
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestPodMutatorSkipsSpreadConstraintWhenNoNodesHaveTopologyLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-ns")
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-ns"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{"gpu.deckhouse.io/pool-a": "pool-a"},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool, node).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1"),
					},
				},
			}},
		},
	}
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}

	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 0 {
		t.Fatalf("expected no topology spread constraints when pool nodes lack topology label")
	}
}

