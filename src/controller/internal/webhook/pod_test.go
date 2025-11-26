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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestPodMutatorAddsAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
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

	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestPodMutatorRejectsMultiplePools(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
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

	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for multiple pools")
	}
}

func TestPodMutatorRejectsConflictingNodeSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{poolLabelKey("pool-a"): "other"},
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

	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for conflicting nodeSelector")
	}
}

func TestPodMutatorRejectsConflictingToleration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{{
				Key:      poolLabelKey("pool-a"),
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

	mutator := newPodMutator(testr.New(t), decoder, nil, nil)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for conflicting toleration")
	}
}

func TestPodMutatorAddsCustomTolerations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	state := moduleconfig.DefaultState()
	state.Settings.Placement.CustomTolerationKeys = []string{"dedicated.apiac.ru", "gpu-role"}
	store := config.NewModuleConfigStore(state)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
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

	mutator := newPodMutator(testr.New(t), decoder, store, nil)
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
	decoder := cradmission.NewDecoder(scheme)

	taintsEnabled := false
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: &taintsEnabled},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
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

	mutator := newPodMutator(testr.New(t), decoder, nil, cl)
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
	decoder := cradmission.NewDecoder(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
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

	mutator := newPodMutator(testr.New(t), decoder, nil, cl)
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
	if val := c.LabelSelector.MatchLabels[poolLabelKey("pool-a")]; val != "pool-a" {
		t.Fatalf("expected label selector with pool key, got %v", c.LabelSelector.MatchLabels)
	}
}

func TestPodMutatorRejectsConflictingSpreadConstraint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
		Spec: v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		Spec: corev1.PodSpec{
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
				TopologyKey:       "topology.kubernetes.io/zone",
				WhenUnsatisfiable: corev1.DoNotSchedule,
				MaxSkew:           1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{poolLabelKey("pool-a"): "other"},
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
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation:       admv1.Create,
		Object:          runtime.RawExtension{Object: &pod},
		Kind:            metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Resource:        metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		RequestKind:     &metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
	}}

	mutator := newPodMutator(testr.New(t), decoder, nil, cl)
	resp := mutator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial for conflicting spread constraint")
	}
}

func TestPodMutatorUsesModuleDefaultSpread(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
		Spec:       v1alpha1.GPUPoolSpec{},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	state := moduleconfig.DefaultState()
	state.Settings.Scheduling.DefaultStrategy = "Spread"
	state.Settings.Scheduling.TopologyKey = "topology.kubernetes.io/zone"
	store := config.NewModuleConfigStore(state)

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
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

	mutator := newPodMutator(testr.New(t), decoder, store, cl)
	resp := mutator.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %v", resp.Result)
	}
	if len(pod.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected topology spread constraint from module defaults")
	}
}
