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

package dpvalidation

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
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
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"},
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

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
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
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStateAssigned,
			NodeName:    "node1",
		},
	}
	// No validator pods -> should fall back to PendingAssignment.
	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
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
		t.Fatalf("expected state PendingAssignment, got %s", updated.Status.State)
	}
}

func TestDPValidationSkipsWhenNoCapacity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName:    "node1",
		},
	}
	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
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
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
		},
	}
	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
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

func TestDPValidationUsesNodeName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"},
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
	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
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
		t.Fatalf("expected Assigned via nodeName, got %s", updated.Status.State)
	}
}
