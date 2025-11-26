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
	"fmt"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestPodMutatorDecodeError(t *testing.T) {
	scheme := runtime.NewScheme()
	decoder := cradmission.NewDecoder(scheme)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation: admv1.Create,
			Object:    runtime.RawExtension{Raw: []byte("not-json")},
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		},
	}
	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected decode error to deny request")
	}
}

func TestPodMutatorNoPoolsAllowed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)
	pod := corev1.Pod{TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"}}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}
	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("empty pod should be allowed")
	}
}

func TestPodMutatorGetPoolNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := newPodMutator(testr.New(t), nil, nil, cl)
	if p := m.getPool(context.Background(), "absent"); p != nil {
		t.Fatalf("expected nil for absent pool")
	}
}

func TestPodMutatorGetPoolOk(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
	m := newPodMutator(testr.New(t), nil, nil, cl)
	if p := m.getPool(context.Background(), "pool"); p == nil {
		t.Fatalf("expected pool found")
	}
}

func TestPoolSchedulingAndTaintsEnabledNoStore(t *testing.T) {
	pool := &v1alpha1.GPUPool{
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:      v1alpha1.GPUPoolSchedulingBinPack,
				TopologyKey:   "k",
				TaintsEnabled: ptr.To(false),
				Taints:        []v1alpha1.GPUPoolTaintSpec{{Key: "gpu"}},
			},
		},
	}
	m := &podMutator{}
	strategy, topo := m.poolScheduling(pool)
	if strategy != string(v1alpha1.GPUPoolSchedulingBinPack) || topo != "k" {
		t.Fatalf("expected scheduling from pool, got %s/%s", strategy, topo)
	}
	if m.poolTaintsEnabled(pool) {
		t.Fatalf("expected taints disabled")
	}
}

func TestPoolSchedulingWithStoreFallback(t *testing.T) {
	state := moduleconfig.State{
		Settings: moduleconfig.Settings{
			Scheduling: moduleconfig.SchedulingSettings{DefaultStrategy: "Spread", TopologyKey: "topo"},
		},
	}
	store := config.NewModuleConfigStore(state)
	m := &podMutator{store: store}
	strategy, topo := m.poolScheduling(nil)
	if strategy != "Spread" || topo != "topo" {
		t.Fatalf("expected store scheduling, got %s/%s", strategy, topo)
	}
	if !m.poolTaintsEnabled(nil) {
		t.Fatalf("expected taints enabled when pool nil")
	}
}

func TestGetPoolNilClient(t *testing.T) {
	m := &podMutator{}
	if p := m.getPool(context.Background(), "any"); p != nil {
		t.Fatalf("expected nil when client is nil")
	}
}

func TestPodMutatorDeniesConflictingToleration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)
	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{{Key: "gpu.deckhouse.io/pool1", Value: "other", Effect: corev1.TaintEffectNoSchedule}},
			Containers:  []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool1"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}
	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial due to conflicting toleration")
	}
}

func TestPodMutatorDeniesConflictingAffinity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)
	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"gpu.deckhouse.io/pool2": "pool2"},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key:      "gpu.deckhouse.io/pool2",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"other"},
							}},
						}},
					},
				},
			},
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool2"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}
	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial due to conflicting affinity")
	}
}

func TestPodMutatorDeniesSpreadConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
				TopologyKey:   "zone",
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu.deckhouse.io/pool3": "other"}},
			}},
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool3"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}

	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1.TypeMeta{Kind: "GPUPool", APIVersion: v1alpha1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "pool3"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingSpread, TopologyKey: "zone"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
	mutator := newPodMutator(testr.New(t), decoder, nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected spread conflict denial")
	}
}

func TestPodMutatorTaintsDisabledSkipsAffinity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool4"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}

	disabled := false
	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1TypeMeta("GPUPool"),
		ObjectMeta: metav1ObjectMeta("pool4"),
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: &disabled},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
	state := moduleconfig.State{Settings: moduleconfig.Settings{Placement: moduleconfig.PlacementSettings{CustomTolerationKeys: []string{"custom"}}}}
	store := config.NewModuleConfigStore(state)

	mutator := newPodMutator(testr.New(t), decoder, store, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected mutation to succeed")
	}
	if len(resp.Patches) == 0 {
		t.Fatalf("expected patches to be generated")
	}
}

func TestPodMutatorMultiplePoolsDenied(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/p1"): resource.MustParse("1"),
						corev1.ResourceName("gpu.deckhouse.io/p2"): resource.MustParse("1"),
					},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}
	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for multiple pools")
	}
}

func TestPodMutatorSpreadSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/p3"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}

	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1TypeMeta("GPUPool"),
		ObjectMeta: metav1ObjectMeta("p3"),
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingSpread, TopologyKey: "zone"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
	mutator := newPodMutator(testr.New(t), decoder, nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected spread mutation to succeed")
	}
}

func TestPodMutatorSpreadWithoutTopologyKey(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)
	pod := corev1.Pod{
		TypeMeta: metav1TypeMeta("Pod"),
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/p5"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}
	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1TypeMeta("GPUPool"),
		ObjectMeta: metav1ObjectMeta("p5"),
		Spec:       v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingSpread}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
	mutator := newPodMutator(testr.New(t), decoder, nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected spread mutation to succeed even without topology key")
	}
}

func TestPodMutatorNodeSelectorConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"gpu.deckhouse.io/p6": "other"},
			Containers:   []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/p6"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}
	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial due to nodeSelector conflict")
	}
}

func TestPodMutatorHandlesObjectPayload(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/p7"): resource.MustParse("1")}}}},
		},
	}
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Object: pod},
	}}
	mutator := newPodMutator(testr.New(t), cradmission.NewDecoder(runtime.NewScheme()), nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected request to be allowed when object provided directly")
	}
}

func TestPodMutatorEmptyRequest(t *testing.T) {
	mutator := newPodMutator(testr.New(t), cradmission.NewDecoder(runtime.NewScheme()), nil, nil)
	resp := mutator.Handle(context.Background(), cradmission.Request{})
	if resp.Allowed {
		t.Fatalf("expected denial on empty admission request")
	}
}

func TestPodMutatorObjectWrongType(t *testing.T) {
	cfg := &corev1.ConfigMap{}
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Object: cfg},
	}}
	mutator := newPodMutator(testr.New(t), cradmission.NewDecoder(runtime.NewScheme()), nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected error for non-pod object")
	}
}

func TestPodMutatorMarshalError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	pod := corev1.Pod{
		TypeMeta: metav1TypeMeta("Pod"),
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/p8"): resource.MustParse("1")}}}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Operation: admv1.Create, Object: runtime.RawExtension{Raw: raw}}}

	mutator := newPodMutator(testr.New(t), cradmission.NewDecoder(scheme), nil, nil)
	orig := jsonMarshal
	defer func() { jsonMarshal = orig }()
	jsonMarshal = func(v any) ([]byte, error) { return nil, fmt.Errorf("boom") }

	resp := mutator.Handle(context.Background(), req)
	if resp.Result == nil || resp.Result.Code != 500 {
		t.Fatalf("expected marshal error, got %+v", resp.Result)
	}
}
