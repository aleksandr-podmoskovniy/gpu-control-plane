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
	"encoding/json"
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

func TestEnsurePoolNodeSelectorAllowsSameValue(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{poolLabelKey(localPoolReq("pool-a")): "pool-a"},
		},
	}
	if err := ensurePoolNodeSelector(pod, poolLabelKey(localPoolReq("pool-a")), "pool-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod.Spec.NodeSelector[poolLabelKey(localPoolReq("pool-a"))] != "pool-a" {
		t.Fatalf("selector value changed unexpectedly")
	}
}

func TestEnsurePoolNodeSelectorInitialisesSelector(t *testing.T) {
	pod := &corev1.Pod{}
	if err := ensurePoolNodeSelector(pod, poolLabelKey(localPoolReq("pool-b")), "pool-b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod.Spec.NodeSelector[poolLabelKey(localPoolReq("pool-b"))] != "pool-b" {
		t.Fatalf("node selector not set")
	}
}

func TestEnsurePoolNodeSelectorConflict(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{poolLabelKey(localPoolReq("pool-c")): "other"},
		},
	}
	if err := ensurePoolNodeSelector(pod, poolLabelKey(localPoolReq("pool-c")), "pool-c"); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestPodMutatorRejectsConflictsWithResolvedPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	ns := enabledNS("default")
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, client)

	makeReq := func(pod corev1.Pod) cradmission.Request {
		raw, _ := json.Marshal(pod)
		return cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw},
		}}
	}

	basePod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool-a"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}

	// conflicting nodeSelector
	pod := basePod
	pod.Spec.NodeSelector = map[string]string{poolLabelKey(localPoolReq("pool-a")): "other"}
	if resp := mutator.Handle(context.Background(), makeReq(pod)); resp.Allowed {
		t.Fatalf("expected denial for conflicting nodeSelector")
	}

	// conflicting toleration
	pod = basePod
	pod.Spec.Tolerations = []corev1.Toleration{{
		Key:      poolLabelKey(localPoolReq("pool-a")),
		Operator: corev1.TolerationOpEqual,
		Value:    "other",
		Effect:   corev1.TaintEffectNoSchedule,
	}}
	if resp := mutator.Handle(context.Background(), makeReq(pod)); resp.Allowed {
		t.Fatalf("expected denial for conflicting toleration")
	}

	// conflicting affinity
	pod = basePod
	pod.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      poolLabelKey(localPoolReq("pool-a")),
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"other"},
					}},
				}},
			},
		},
	}
	if resp := mutator.Handle(context.Background(), makeReq(pod)); resp.Allowed {
		t.Fatalf("expected denial for conflicting affinity")
	}
}

func TestResolvePoolVariants(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	stateful := v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingBinPack}}

	nsPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns1"}, Spec: stateful}
	clusterPool := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-b"}, Spec: v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{TopologyKey: "topology.kubernetes.io/zone"}}}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nsPool, clusterPool).Build()

	// namespaced pool found
	pool, err := resolvePoolByRequest(context.Background(), client, localPoolReq("pool-a"), "ns1")
	if err != nil {
		t.Fatalf("expected namespaced pool, got error: %v", err)
	}
	if pool.Spec.Scheduling.Strategy != stateful.Scheduling.Strategy {
		t.Fatalf("unexpected spec from namespaced pool")
	}

	// cluster pool is resolved via cluster prefix
	pool, err = resolvePoolByRequest(context.Background(), client, clusterPoolReq("pool-b"), "ns1")
	if err != nil {
		t.Fatalf("expected cluster pool, got error: %v", err)
	}
	if pool.Name != "pool-b" || pool.Spec.Scheduling.TopologyKey != "topology.kubernetes.io/zone" {
		t.Fatalf("unexpected cluster pool data: %+v", pool)
	}

	// client missing
	if _, err := resolvePoolByRequest(context.Background(), nil, localPoolReq("pool-a"), "ns1"); err == nil {
		t.Fatalf("expected error when client is nil")
	}
	// namespace missing
	if _, err := resolvePoolByRequest(context.Background(), client, localPoolReq("pool-a"), ""); err == nil {
		t.Fatalf("expected error for empty namespace")
	}
	// not found
	if _, err := resolvePoolByRequest(context.Background(), client, localPoolReq("absent"), "ns1"); err == nil {
		t.Fatalf("expected not found error")
	}
}

func TestPodValidator(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-ns")
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "gpu-ns"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
			Conditions: []metav1.Condition{{
				Type:   "Configured",
				Status: metav1.ConditionTrue,
			}},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	validator := newPodValidator(testr.New(t), cl)

	makeReq := func(pod corev1.Pod) cradmission.Request {
		raw, _ := json.Marshal(pod)
		return cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw},
		}}
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}
	pod.Namespace = "gpu-ns"
	resp := validator.Handle(context.Background(), makeReq(pod))
	if !resp.Allowed {
		t.Fatalf("expected allowed, got %v", resp)
	}

	// ns without label
	nsNoLabel := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "plain"}}
	cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nsNoLabel, pool.DeepCopy()).Build()
	validator = newPodValidator(testr.New(t), cl)
	pod.Namespace = "plain"
	resp = validator.Handle(context.Background(), makeReq(pod))
	if resp.Allowed {
		t.Fatalf("expected deny for ns without label")
	}

	// missing pool
	cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	validator = newPodValidator(testr.New(t), cl)
	pod.Namespace = "gpu-ns"
	resp = validator.Handle(context.Background(), makeReq(pod))
	if resp.Allowed {
		t.Fatalf("expected deny for missing pool")
	}
}

func TestPodValidatorNoPools(t *testing.T) {
	validator := newPodValidator(testr.New(t), nil)
	pod := corev1.Pod{}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}
	resp := validator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed when no gpu resources requested")
	}
}

func TestPodValidatorMultiplePools(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	ns := enabledNS("ns")
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	validator := newPodValidator(testr.New(t), cl)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool"):  *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceName("gpu.deckhouse.io/pool2"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}
	resp := validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for multiple pools")
	}
}
