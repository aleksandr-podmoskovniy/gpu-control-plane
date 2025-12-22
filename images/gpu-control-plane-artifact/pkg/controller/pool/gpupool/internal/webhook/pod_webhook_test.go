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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
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
	store := moduleconfig.NewModuleConfigStore(state)
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
}
