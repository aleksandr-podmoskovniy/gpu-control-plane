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
)

func TestPodValidatorGVK(t *testing.T) {
	v := &podValidator{}
	gvk := v.GVK()
	if gvk.Kind != "Pod" || gvk.Version != "v1" {
		t.Fatalf("unexpected GVK: %v", gvk)
	}
}

func TestPodValidatorHandleNoPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)
	v := newPodValidator(testr.New(t), decoder, nil, nil)

	pod := corev1.Pod{}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw, Object: &pod},
		},
	}
	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed when no gpu pool requested")
	}
}

func TestPodValidatorNamespaceNotEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gpu-team"}}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw, Object: &pod},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	v := newPodValidator(testr.New(t), decoder, nil, cl)
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial when namespace not enabled")
	}
}

func TestPodValidatorClusterPoolResolve(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "shared"},
		Spec:       v1alpha1.GPUPoolSpec{},
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("cluster.gpu.deckhouse.io/shared"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw, Object: &pod},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, clusterPool).Build()
	v := newPodValidator(testr.New(t), decoder, nil, cl)
	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed for cluster pool pod, got %v", resp.Result)
	}
}

func TestPodValidatorResolvePoolErrors(t *testing.T) {
	v := &podValidator{}
	if _, err := v.resolvePool(context.Background(), poolRequest{name: "a"}, ""); err == nil {
		t.Fatalf("expected error without client")
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v.client = cl
	if _, err := v.resolvePool(context.Background(), poolRequest{name: "a"}, ""); err == nil {
		t.Fatalf("expected error when namespace empty for namespaced pool")
	}

	if _, err := v.resolvePool(context.Background(), poolRequest{name: "a", isCluster: true}, ""); err == nil {
		t.Fatalf("expected error when cluster pool missing")
	}
}

func TestPodValidatorMultiplePoolsAndMissingPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/a"): resource.MustParse("1"),
						corev1.ResourceName("gpu.deckhouse.io/b"): resource.MustParse("1"),
					},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw, Object: &pod}}}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	v := newPodValidator(testr.New(t), decoder, nil, cl)
	if resp := v.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial for multiple pools")
	}

	// single pool but missing in cluster
	pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		corev1.ResourceName("gpu.deckhouse.io/missing"): resource.MustParse("1"),
	}
	raw, _ = json.Marshal(pod)
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw, Object: &pod}}}
	if resp := v.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial when pool not found")
	}
}

func TestPodValidatorHandleSwitchBranches(t *testing.T) {
	v := &podValidator{}
	// empty request
	resp := v.Handle(context.Background(), cradmission.Request{})
	if resp.Allowed {
		t.Fatalf("expected denial on empty request")
	}

	// non-pod object
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: &corev1.Namespace{}}}}
	resp = v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected error for non-pod object")
	}

	// object branch with Pod
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
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
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	v = newPodValidator(testr.New(t), decoder, nil, cl)
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	if resp := v.Handle(context.Background(), req); !resp.Allowed {
		t.Fatalf("expected allowed in object branch, got %v", resp.Result)
	}

	// invalid raw json triggers error branch
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: []byte(`{invalid}`)}}}
	resp = v.Handle(context.Background(), req)
	if resp.Allowed || resp.Result == nil || resp.Result.Code != 422 {
		t.Fatalf("expected 422 on invalid raw payload, got %+v", resp)
	}
}

func TestPodValidatorNamespaceNotFoundAndNamespacedPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
				},
			}},
		},
	}
	raw, _ := json.Marshal(pod)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: raw, Object: &pod}}}

	// namespace missing
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := newPodValidator(testr.New(t), decoder, nil, cl)
	if resp := v.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial when namespace missing")
	}

	// namespace present and enabled, pool present
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"}}
	cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	v = newPodValidator(testr.New(t), decoder, nil, cl)
	if resp := v.Handle(context.Background(), req); !resp.Allowed {
		t.Fatalf("expected allowed when namespace/pool exist, got %v", resp.Result)
	}
}
