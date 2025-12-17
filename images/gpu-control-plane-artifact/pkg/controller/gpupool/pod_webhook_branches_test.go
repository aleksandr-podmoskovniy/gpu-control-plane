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
	"encoding/json"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

type failingNodeListClient struct {
	client.Client
	err error
}

func (c *failingNodeListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.NodeList); ok {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

func TestPodMutatorDeniesWhenPoolNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-ns")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}

	if resp := mutator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial when pool is missing")
	}
}

func TestPodMutatorDeniesOnEnsureNodeTolerationsError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-ns")
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-ns"},
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	cl := &failingNodeListClient{Client: base, err: apierrors.NewBadRequest("list nodes failed")}
	mutator := newPodMutator(testr.New(t), nil, cl)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}

	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed || resp.Result == nil || resp.Result.Code != 403 {
		t.Fatalf("expected denial on ensureNodeTolerations error, got %+v", resp.Result)
	}
}

func TestPodMutatorDeniesOnTopologyLabelPresentError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-ns")
	taintsEnabled := false
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-ns"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				TaintsEnabled: &taintsEnabled,
				Strategy:      v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey:   "topology.kubernetes.io/zone",
			},
		},
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	cl := &failingNodeListClient{Client: base, err: apierrors.NewBadRequest("list nodes failed")}
	mutator := newPodMutator(testr.New(t), nil, cl)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}

	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial on topologyLabelPresent error")
	}
}

func TestPodMutatorSpreadWithoutClientPropagatesSpreadConflict(t *testing.T) {
	state := v1alpha1.GPUPoolSchedulingSpread
	store := configStoreWithScheduling(string(state), "topology.kubernetes.io/zone")
	mutator := newPodMutator(testr.New(t), store, nil)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
				TopologyKey:   "topology.kubernetes.io/zone",
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu.deckhouse.io/pool-a": "other"}},
			}},
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}

	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial on spread conflict without client")
	}
}

func configStoreWithScheduling(strategy, topologyKey string) *moduleconfig.ModuleConfigStore {
	state := moduleconfig.DefaultState()
	state.Settings.Scheduling.DefaultStrategy = strategy
	state.Settings.Scheduling.TopologyKey = topologyKey
	return moduleconfig.NewModuleConfigStore(state)
}

func TestCollectPoolNodeTaintsClusterPrefix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}
	poolKey := clusterPoolResourcePrefix + pool.Name
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{poolKey: "pool-a"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "a", Value: "", Effect: corev1.TaintEffectNoSchedule},
				{Key: "a", Value: "", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	d := &PodDefaulter{client: cl}

	taints, err := d.collectPoolNodeTaints(context.Background(), pool)
	if err != nil {
		t.Fatalf("collectPoolNodeTaints: %v", err)
	}
	if len(taints) != 1 {
		t.Fatalf("expected deduplicated taints, got %v", taints)
	}
}

func TestTopologyLabelPresentBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n1",
			Labels: map[string]string{
				"gpu.deckhouse.io/pool-a":         "pool-a",
				"topology.kubernetes.io/zone":     "z1",
				"topology.kubernetes.io/hostname": "n1",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	d := &PodDefaulter{client: cl}

	ok, err := d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "")
	if err != nil || ok {
		t.Fatalf("expected empty topologyKey to return (false,nil), got (%v,%v)", ok, err)
	}

	ok, err = d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "topology.kubernetes.io/zone")
	if err != nil || !ok {
		t.Fatalf("expected topology label present, got (%v,%v)", ok, err)
	}

	ok, err = d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "topology.kubernetes.io/region")
	if err != nil || ok {
		t.Fatalf("expected topology label missing to return false, got (%v,%v)", ok, err)
	}

	// Empty node list -> ok=true (unknown yet).
	d = &PodDefaulter{client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ok, err = d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "topology.kubernetes.io/zone")
	if err != nil || !ok {
		t.Fatalf("expected empty list to return ok=true, got (%v,%v)", ok, err)
	}

	// list error
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	d = &PodDefaulter{client: &failingNodeListClient{Client: base, err: apierrors.NewBadRequest("list failed")}}
	ok, err = d.topologyLabelPresent(context.Background(), "gpu.deckhouse.io/pool-a", "pool-a", "topology.kubernetes.io/zone")
	if err == nil || ok {
		t.Fatalf("expected list error to be returned")
	}
}

func TestEnsureNodeTolerationsAddsEqualAndExists(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	poolKey := localPoolResourcePrefix + pool.Name
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{poolKey: "pool-a"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "value", Value: "x", Effect: corev1.TaintEffectNoSchedule},
				{Key: "exists", Value: "", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	d := &PodDefaulter{client: cl}

	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Tolerations: []corev1.Toleration{{Key: "value", Operator: corev1.TolerationOpEqual, Value: "x", Effect: corev1.TaintEffectNoSchedule}},
	}}
	if err := d.ensureNodeTolerations(context.Background(), pod, pool); err != nil {
		t.Fatalf("ensureNodeTolerations: %v", err)
	}
	if len(pod.Spec.Tolerations) != 2 {
		t.Fatalf("expected one new toleration appended, got %v", pod.Spec.Tolerations)
	}
	if !hasToleration(pod.Spec.Tolerations, "exists") {
		t.Fatalf("expected Exists toleration added")
	}
}

func TestPodMutatorAllowsSpreadWithoutClientWhenNoConflict(t *testing.T) {
	state := v1alpha1.GPUPoolSchedulingSpread
	store := configStoreWithScheduling(string(state), "topology.kubernetes.io/zone")
	mutator := newPodMutator(testr.New(t), store, nil)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed without client, got %v", resp.Result)
	}
}
