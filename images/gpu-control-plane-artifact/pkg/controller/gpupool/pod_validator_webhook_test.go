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
	"net/http"
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

func TestPodValidatorHandleNoPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	v := newPodValidator(testr.New(t), nil)

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
	v := newPodValidator(testr.New(t), cl)
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial when namespace not enabled")
	}
}

func TestPodValidatorClusterPoolResolve(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "shared"},
		Spec:       v1alpha1.GPUPoolSpec{},
		Status: v1alpha1.GPUPoolStatus{
			Capacity:   v1alpha1.GPUPoolCapacityStatus{Total: 1},
			Conditions: []metav1.Condition{{Type: "Configured", Status: metav1.ConditionTrue}},
		},
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
	v := newPodValidator(testr.New(t), cl)
	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed for cluster pool pod, got %v", resp.Result)
	}
}

func TestPodValidatorDeniesWhenCapacityExceeded(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "shared"},
		Spec:       v1alpha1.GPUPoolSpec{},
		Status: v1alpha1.GPUPoolStatus{
			Capacity:   v1alpha1.GPUPoolCapacityStatus{Total: 1},
			Conditions: []metav1.Condition{{Type: "Configured", Status: metav1.ConditionTrue}},
		},
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "gpu-team"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("cluster.gpu.deckhouse.io/shared"): resource.MustParse("2")},
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
	v := newPodValidator(testr.New(t), cl)
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial when requested exceeds total")
	}
}

func TestPodValidatorAllowsWhenCapacityIsZero(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "shared"},
		Spec:       v1alpha1.GPUPoolSpec{},
		Status: v1alpha1.GPUPoolStatus{
			Capacity:   v1alpha1.GPUPoolCapacityStatus{Total: 0},
			Conditions: []metav1.Condition{{Type: "Configured", Status: metav1.ConditionTrue}},
		},
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
	v := newPodValidator(testr.New(t), cl)
	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed when capacity is zero, got %v", resp.Result)
	}
}

func TestPodValidatorDeniesWhenPoolNotConfigured(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
			Conditions: []metav1.Condition{{
				Type:    "Configured",
				Status:  metav1.ConditionFalse,
				Reason:  "NameCollision",
				Message: "pool configuration is invalid",
			}},
		},
	}
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

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	v := newPodValidator(testr.New(t), cl)
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected denial when pool is not configured")
	}
}

func TestRequestedResourcesUsesInitContainerMax(t *testing.T) {
	pool := poolRequest{name: "pool-a", keyPrefix: localPoolResourcePrefix}

	t.Run("init-container-dominates", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("3")},
					},
				}},
				Containers: []corev1.Container{
					{Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
					}},
					{Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
					}},
				},
			},
		}

		if got := requestedResources(pod, pool); got != 3 {
			t.Fatalf("unexpected requestedResources(): got %d, want %d", got, 3)
		}
	})

	t.Run("containers-sum-dominates", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("1")},
					},
				}},
				Containers: []corev1.Container{
					{Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("2")},
					}},
					{Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool-a"): resource.MustParse("2")},
					}},
				},
			},
		}

		if got := requestedResources(pod, pool); got != 4 {
			t.Fatalf("unexpected requestedResources(): got %d, want %d", got, 4)
		}
	})
}

func TestPodValidatorResolvePoolErrors(t *testing.T) {
	if _, err := resolvePoolByRequest(context.Background(), nil, poolRequest{name: "a", keyPrefix: localPoolResourcePrefix}, "ns"); err == nil {
		t.Fatalf("expected error without client")
	}
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	if _, err := resolvePoolByRequest(context.Background(), cl, poolRequest{name: "a", keyPrefix: localPoolResourcePrefix}, ""); err == nil {
		t.Fatalf("expected error when namespace empty for namespaced pool")
	}
}

func TestPodValidatorMultiplePoolsAndMissingPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
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
	v := newPodValidator(testr.New(t), cl)
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
	v := newPodValidator(testr.New(t), nil)
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
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
			Conditions: []metav1.Condition{{
				Type:   "Configured",
				Status: metav1.ConditionTrue,
			}},
		},
	}
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
	v = newPodValidator(testr.New(t), cl)
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Object: pod}}}
	if resp := v.Handle(context.Background(), req); !resp.Allowed {
		t.Fatalf("expected allowed in object branch, got %v", resp.Result)
	}

	// invalid raw json triggers error branch
	req = cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Object: runtime.RawExtension{Raw: []byte(`{invalid}`)}}}
	resp = v.Handle(context.Background(), req)
	if resp.Allowed || resp.Result == nil || resp.Result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on invalid raw payload, got %+v", resp)
	}
}

func TestPodValidatorNamespaceNotFoundAndNamespacedPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

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
	v := newPodValidator(testr.New(t), cl)
	if resp := v.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial when namespace missing")
	}

	// namespace present and enabled, pool present
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "gpu-team",
		Labels: map[string]string{"gpu.deckhouse.io/enabled": "true"},
	}}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
			Conditions: []metav1.Condition{{
				Type:   "Configured",
				Status: metav1.ConditionTrue,
			}},
		},
	}
	cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pool).Build()
	v = newPodValidator(testr.New(t), cl)
	if resp := v.Handle(context.Background(), req); !resp.Allowed {
		t.Fatalf("expected allowed when namespace/pool exist, got %v", resp.Result)
	}
}
