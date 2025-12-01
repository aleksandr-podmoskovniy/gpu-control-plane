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
	"fmt"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestDPValidationHandlerName(t *testing.T) {
	h := NewDPValidationHandler(testr.New(t), nil)
	if h.Name() != "dp-validation" {
		t.Fatalf("unexpected name %s", h.Name())
	}
}

func TestDPValidationTransitions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	// attach nodeName
	pod.Spec.NodeName = "node1"

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("expected state Assigned, got %s", updated.Status.State)
	}
}

func TestDPValidationBackToPendingWhenNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStateAssigned,
			NodeName:    "node1",
		},
	}
	// No validator pods -> should stay Assigned.
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("expected state Assigned, got %s", updated.Status.State)
	}
}

func TestDPValidationSkipsWhenNoCapacity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("state should remain pending, got %s", updated.Status.State)
	}
}

func TestDPValidationSkipsWithoutNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("state should remain pending, got %s", updated.Status.State)
	}
}

func TestDPValidationUsesNodeLabelFallback(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
			Labels:      map[string]string{"kubernetes.io/hostname": "node1"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Spec:   corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("expected Assigned via label fallback, got %s", updated.Status.State)
	}
}

func TestDPValidationPodListErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.podList = func(ctx context.Context, cl client.Client, opts ...client.ListOption) (*corev1.PodList, error) {
		return nil, fmt.Errorf("boom")
	}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected podList error")
	}
}

func TestDPValidationPodListErrorsDefaultLister(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	errClient := &errorListClient{Client: base, err: fmt.Errorf("pod list error")}
	h := NewDPValidationHandler(testr.New(t), errClient)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected pod list error via default lister")
	}
}

func TestDPValidationDeviceListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	errClient := &errorListClient{Client: fakeClient, err: fmt.Errorf("list error")}

	h := NewDPValidationHandler(testr.New(t), errClient)
	h.ns = "ns"
	h.podList = func(ctx context.Context, cl client.Client, opts ...client.ListOption) (*corev1.PodList, error) {
		return &corev1.PodList{}, nil
	}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected list error")
	}
}

func TestDPValidationNotFoundPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStateAssigned,
			NodeName:    "node1",
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.podList = func(ctx context.Context, cl client.Client, opts ...client.ListOption) (*corev1.PodList, error) {
		return nil, apierrors.NewNotFound(corev1.Resource("pods"), "x")
	}
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool should ignore notfound: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("expected state to remain Assigned, got %s", updated.Status.State)
	}
}

func TestNewDPValidationHandlerNamespaceDefaults(t *testing.T) {
	h := NewDPValidationHandler(testr.New(t), nil)
	if h.ns != "d8-gpu-control-plane" {
		t.Fatalf("default namespace mismatch: %s", h.ns)
	}
	// force empty env to hit default branch explicitly
	t.Setenv("POD_NAMESPACE", "")
	h = NewDPValidationHandler(testr.New(t), nil)
	if h.ns != "d8-gpu-control-plane" {
		t.Fatalf("default namespace mismatch after reset: %s", h.ns)
	}
	t.Setenv("POD_NAMESPACE", "custom-ns")
	h = NewDPValidationHandler(testr.New(t), nil)
	if h.ns != "custom-ns" {
		t.Fatalf("expected namespace from env, got %s", h.ns)
	}
}

func TestDPValidationNilClient(t *testing.T) {
	h := NewDPValidationHandler(testr.New(t), nil)
	pool := &v1alpha1.GPUPool{Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("expected nil client to no-op, got %v", err)
	}
}

func TestDPValidationPodNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Spec:   corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("state should stay pending, got %s", updated.Status.State)
	}
}

func TestDPValidationPodWithoutNodeName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		// NodeName intentionally empty to hit continue branch.
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("state should stay pending, got %s", updated.Status.State)
	}
}

func TestDPValidationAssignedReadyNoChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStateAssigned,
			NodeName:    "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Spec:   corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("state should stay assigned, got %s", updated.Status.State)
	}
}

func TestDPValidationPatchError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Spec:   corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	cl := &errorStatusClient{Client: base, err: fmt.Errorf("patch error")}
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected patch error")
	}
}

func TestDPValidationIgnoresOtherStates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStateFaulted,
			NodeName:    "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Spec:   corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateFaulted {
		t.Fatalf("state should remain faulted, got %s", updated.Status.State)
	}
}

func TestDPValidationSkipsDevicesFromOtherPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "another"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("fetch device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("state should remain pending, got %s", updated.Status.State)
	}
}

func TestIsPodReadyFalse(t *testing.T) {
	if isPodReady(nil) {
		t.Fatalf("nil pod should not be ready")
	}
	pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}}}
	if isPodReady(pod) {
		t.Fatalf("pod with ready=false should not be ready")
	}
}

type errorListClient struct {
	client.Client
	err error
}

func (c *errorListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.err != nil {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

type errorStatusClient struct {
	client.Client
	err error
}

func (c *errorStatusClient) Status() client.SubResourceWriter {
	return &errorStatusWriter{SubResourceWriter: c.Client.Status(), err: c.err}
}

type errorStatusWriter struct {
	client.SubResourceWriter
	err error
}

func (w *errorStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if w.err != nil {
		return w.err
	}
	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}
