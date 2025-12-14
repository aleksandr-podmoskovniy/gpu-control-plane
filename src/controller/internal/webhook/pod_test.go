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
	"errors"
	"net/http"
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

func localPoolReq(name string) poolRequest {
	return poolRequest{name: name, keyPrefix: localPoolResourcePrefix}
}

func clusterPoolReq(name string) poolRequest {
	return poolRequest{name: name, keyPrefix: clusterPoolResourcePrefix}
}

func enabledNS(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
		},
	}
}

func TestPodMutatorAddsAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "d8-gpu-control-plane",
		},
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
	rawPod, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestPodMutatorRejectsMultiplePools(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"}}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)

	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "d8-gpu-control-plane"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool-a"): *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceName("gpu.deckhouse.io/pool-b"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}
	rawPod, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for multiple pools")
	}
}

func TestPodMutatorRejectsConflictingNodeSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{poolLabelKey(localPoolReq("pool-a")): "other"},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	mutator := newPodMutator(testr.New(t), nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for conflicting nodeSelector")
	}
}

func TestPodMutatorRejectsConflictingToleration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{{
				Key:      poolLabelKey(localPoolReq("pool-a")),
				Operator: corev1.TolerationOpEqual,
				Value:    "other",
				Effect:   corev1.TaintEffectNoSchedule,
			}},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	mutator := newPodMutator(testr.New(t), nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for conflicting toleration")
	}
}

func TestPodMutatorNamespaceNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "missing"},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	// client without namespace object
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial when namespace missing")
	}
}

func TestPodMutatorTaintsDisabledSkipTolerations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-team")
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: ptr.To(false)},
		},
	}
	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: rawPod, Object: &pod}}}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed for taints-disabled pool, got %v", resp.Result)
	}
	if len(pod.Spec.Tolerations) != 0 {
		t.Fatalf("expected tolerations untouched when taints disabled")
	}
}

func TestPodMutatorNamespaceNotEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gpu-team"}}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"}}
	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: rawPod, Object: &pod}}}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial when namespace not enabled")
	}
}

func TestPodMutatorNoGPUResources(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	mutator := newPodMutator(testr.New(t), nil, nil)

	pod := corev1.Pod{}
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: rawPod, Object: &pod}}}
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed when no gpu pool requested")
	}
}

func TestPodMutatorObjectBranch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-team")
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed when object branch used, got %v", resp.Result)
	}
}

func TestPodMutatorSpreadAndCustomTolerations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := enabledNS("gpu-team")
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"}}
	state := moduleconfig.State{
		Settings: moduleconfig.Settings{
			Placement:  moduleconfig.PlacementSettings{CustomTolerationKeys: []string{"dedicated.apiac.ru"}},
			Scheduling: moduleconfig.SchedulingSettings{DefaultStrategy: "Spread", TopologyKey: "topology.kubernetes.io/zone"},
		},
	}
	store := config.NewModuleConfigStore(state)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), store, cl)

	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got %v", resp.Result)
	}
	if !hasToleration(pod.Spec.Tolerations, "dedicated.apiac.ru") {
		t.Fatalf("expected custom toleration injected, got %v", pod.Spec.Tolerations)
	}
	foundSpread := false
	for _, sc := range pod.Spec.TopologySpreadConstraints {
		if sc.TopologyKey == "topology.kubernetes.io/zone" {
			foundSpread = true
		}
	}
	if !foundSpread {
		t.Fatalf("expected spread constraint added")
	}
}

func TestPodMutatorInvalidObjectAndEmptyRequest(t *testing.T) {
	mutator := newPodMutator(testr.New(t), nil, nil)

	// non-pod object should error
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: &corev1.Namespace{}}}}
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed || resp.Result == nil || resp.Result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected errored response for non-pod object, got %+v", resp)
	}

	// empty request should be denied
	resp = mutator.Handle(context.Background(), cradmission.Request{})
	if resp.Allowed {
		t.Fatalf("expected denial for empty request")
	}
}

func TestPodMutatorHandleErrorBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	ns := enabledNS("gpu-team")
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"}}
	basePod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
					},
				}},
			},
		}
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	mutator := newPodMutator(testr.New(t), nil, cl)

	// conflicting nodeSelector
	pod := basePod()
	pod.Spec.NodeSelector = map[string]string{"gpu.deckhouse.io/pool-a": "other"}
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	if resp := mutator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial for conflicting nodeSelector")
	}

	// conflicting toleration
	pod = basePod()
	pod.Spec.Tolerations = []corev1.Toleration{{Key: "gpu.deckhouse.io/pool-a", Value: "other", Effect: corev1.TaintEffectNoSchedule}}
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	if resp := mutator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial for conflicting toleration")
	}

	// conflicting affinity
	pod = basePod()
	pod.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: "gpu.deckhouse.io/pool-a", Operator: corev1.NodeSelectorOpIn, Values: []string{"other"}},
					},
				}},
			},
		},
	}
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	if resp := mutator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial for conflicting affinity")
	}

	// marshal error path
	origMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { jsonMarshal = origMarshal }()
	pod = basePod()
	raw, _ := json.Marshal(pod)
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw}}}
	if resp := mutator.Handle(context.Background(), req); resp.Allowed || resp.Result.Code != http.StatusInternalServerError {
		t.Fatalf("expected errored response on marshal failure, got %+v", resp)
	}
}

func TestPodMutatorAddsCustomTolerations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	state := moduleconfig.DefaultState()
	state.Settings.Placement.CustomTolerationKeys = []string{"dedicated.apiac.ru", "gpu-role"}
	store := config.NewModuleConfigStore(state)
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "d8-gpu-control-plane"},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	mutator := newPodMutator(testr.New(t), store, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
	if len(resp.Patches) == 0 {
		t.Fatalf("expected patches to include custom tolerations")
	}
}

func TestPodMutatorSkipsPoolTolerationWhenTaintsDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	taintsEnabled := false
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: &taintsEnabled},
		},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "d8-gpu-control-plane",
		},
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
	rawPod, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation:       admv1.Create,
			Object:          runtime.RawExtension{Raw: rawPod, Object: &pod},
			Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
	if len(pod.Spec.Tolerations) != 0 {
		t.Fatalf("pool toleration should be skipped when taints disabled")
	}
}

func TestPodMutatorAddsSpreadConstraint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
			},
		},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "d8-gpu-control-plane",
		},
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
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation:       admv1.Create,
		Object:          runtime.RawExtension{Object: &pod},
		Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
	}}

	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected topology spread constraint to be added")
	}
	c := pod.Spec.TopologySpreadConstraints[0]
	if c.TopologyKey != "topology.kubernetes.io/zone" || c.WhenUnsatisfiable != corev1.DoNotSchedule || c.MaxSkew != 1 {
		t.Fatalf("unexpected spread constraint %+v", c)
	}
	if val := c.LabelSelector.MatchLabels[poolLabelKey(localPoolReq("pool-a"))]; val != "pool-a" {
		t.Fatalf("expected label selector with pool key, got %v", c.LabelSelector.MatchLabels)
	}
}

func TestPodMutatorRejectsConflictingSpreadConstraint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
			},
		},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "d8-gpu-control-plane"},
		Spec: corev1.PodSpec{
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
				TopologyKey:       "topology.kubernetes.io/zone",
				WhenUnsatisfiable: corev1.DoNotSchedule,
				MaxSkew:           1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{poolLabelKey(localPoolReq("pool-a")): "other"},
				},
			}},
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
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation:       admv1.Create,
		Object:          runtime.RawExtension{Raw: raw},
		Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
	}}

	mutator := newPodMutator(testr.New(t), nil, cl)
	if _, err := resolvePoolByRequest(context.Background(), cl, localPoolReq("pool-a"), "d8-gpu-control-plane"); err != nil {
		t.Fatalf("expected pool to be retrievable: %v", err)
	}
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for conflicting spread constraint")
	}
}

func TestPodMutatorUsesModuleDefaultSpread(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
		Spec:       v1alpha1.GPUPoolSpec{},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	state := moduleconfig.DefaultState()
	state.Settings.Scheduling.DefaultStrategy = "Spread"
	state.Settings.Scheduling.TopologyKey = "topology.kubernetes.io/zone"
	store := config.NewModuleConfigStore(state)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "d8-gpu-control-plane",
		},
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
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation:       admv1.Create,
		Object:          runtime.RawExtension{Object: &pod},
		Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
	}}

	mutator := newPodMutator(testr.New(t), store, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected topology spread constraint from module defaults")
	}
}

func TestPodMutatorHandlesInvalidJSON(t *testing.T) {
	mutator := newPodMutator(testr.New(t), nil, nil)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: []byte("not-a-pod")},
	}}
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestPodMutatorSpreadWithoutTopology(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "d8-gpu-control-plane"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy: v1alpha1.GPUPoolSchedulingSpread,
			},
		},
	}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "d8-gpu-control-plane"},
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
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Object: runtime.RawExtension{Object: &pod},
	}}
	mutator := newPodMutator(testr.New(t), nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got %v", resp.Result)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 0 {
		t.Fatalf("topology spread constraint should be skipped without topology key")
	}
}

func TestPodMutatorHandlesMarshalError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	ns := enabledNS("d8-gpu-control-plane")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()

	pod := corev1.Pod{
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
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: raw},
	}}

	mutator := newPodMutator(testr.New(t), nil, cl)
	origMarshal := jsonMarshal
	t.Cleanup(func() { jsonMarshal = origMarshal })
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }

	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected marshal error to deny request")
	}
}

func TestPodMutatorHandleRequestShapeEdgeCases(t *testing.T) {
	mutator := newPodMutator(testr.New(t), nil, nil)

	// empty request
	resp := mutator.Handle(context.Background(), cradmission.Request{})
	if resp.Allowed {
		t.Fatalf("expected denial for empty request")
	}

	// non-pod object
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Object: runtime.RawExtension{Object: &corev1.ConfigMap{}},
	}}
	resp = mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected error for non-pod object")
	}

	// request without gpu resources
	pod := &corev1.Pod{}
	raw, _ := json.Marshal(pod)
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw, Object: pod}}}
	resp = mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed for pod without gpu request")
	}
	if len(resp.Patches) != 0 {
		t.Fatalf("expected no patches when no pools requested")
	}
}

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
