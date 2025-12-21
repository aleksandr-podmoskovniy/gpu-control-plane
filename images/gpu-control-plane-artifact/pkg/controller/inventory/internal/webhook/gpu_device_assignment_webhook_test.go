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
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestGPUDeviceAssignmentValidatorNamespacedPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	poolObj := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team-local"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"inv-1"}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(poolObj).Build()
	validator := NewGPUDeviceAssignmentValidator(testr.New(t), cl)

	oldDevice := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a"},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-1",
			State:       v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}
	newDevice := oldDevice.DeepCopy()
	newDevice.Annotations = map[string]string{namespacedAssignmentAnnotation: "pool-a"}

	if _, err := validator.ValidateUpdate(context.Background(), oldDevice, newDevice); err != nil {
		t.Fatalf("expected allowed assignment, got %v", err)
	}

	newDevice.Status.InventoryID = "inv-2"
	if _, err := validator.ValidateUpdate(context.Background(), oldDevice, newDevice); err == nil {
		t.Fatalf("expected denial for selector mismatch")
	}

	newDevice.Status.InventoryID = "inv-1"
	newDevice.Annotations[namespacedAssignmentAnnotation] = "absent"
	if _, err := validator.ValidateUpdate(context.Background(), oldDevice, newDevice); err == nil {
		t.Fatalf("expected denial for missing pool")
	}

	assigned := oldDevice.DeepCopy()
	assigned.Annotations = map[string]string{namespacedAssignmentAnnotation: "pool-a"}
	without := oldDevice.DeepCopy()
	if _, err := validator.ValidateUpdate(context.Background(), assigned, without); err != nil {
		t.Fatalf("expected allowed when assignment removed: %v", err)
	}
}

func TestGPUDeviceAssignmentValidatorAllowsCreateWithoutAssignment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev"}}
	if _, err := validator.ValidateCreate(context.Background(), device); err != nil {
		t.Fatalf("expected allowed on create without assignment, got %v", err)
	}
}

func TestGPUDeviceAssignmentValidatorAllowsUpdateWhenAssignmentUnchanged(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())

	oldDevice := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{namespacedAssignmentAnnotation: "pool-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStatePendingAssignment},
	}
	newDevice := oldDevice.DeepCopy()
	newDevice.Labels = map[string]string{"example": "label"}

	if _, err := validator.ValidateUpdate(context.Background(), oldDevice, newDevice); err != nil {
		t.Fatalf("expected allowed update when assignment unchanged, got %v", err)
	}
}

func TestGPUDeviceAssignmentValidatorAmbiguousPoolName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	poolA := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns-a"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	poolB := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns-b"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(poolA, poolB).Build()

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), cl)
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{namespacedAssignmentAnnotation: "pool-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:       v1alpha1.GPUDeviceStateReady,
			InventoryID: "inv-1",
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}
	if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
		t.Fatalf("expected denial for ambiguous pool name")
	}
}

func TestGPUDeviceAssignmentValidatorClusterPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"inv-1"}},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterPool).Build()
	validator := NewGPUDeviceAssignmentValidator(testr.New(t), cl)

	oldDevice := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a"},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-1",
			State:       v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}
	newDevice := oldDevice.DeepCopy()
	newDevice.Annotations = map[string]string{clusterAssignmentAnnotation: "cluster-a"}

	if _, err := validator.ValidateUpdate(context.Background(), oldDevice, newDevice); err != nil {
		t.Fatalf("expected allowed assignment to ClusterGPUPool, got %v", err)
	}
}

func TestGPUDeviceAssignmentValidatorRejectsBothAssignments(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dev-a",
			Annotations: map[string]string{
				namespacedAssignmentAnnotation: "pool-a",
				clusterAssignmentAnnotation:    "cluster-a",
			},
		},
	}
	if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
		t.Fatalf("expected denial when both assignment annotations are set")
	}
}

type failingGetClient struct {
	client.Client
}

func (f *failingGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return apierrors.NewBadRequest("boom")
}

func TestGPUDeviceAssignmentValidatorClientError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{clusterAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State: v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), &failingGetClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()})
	_, err := validator.ValidateCreate(context.Background(), device)
	if err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected bad request error, got %v", err)
	}
}

func TestGPUDeviceAssignmentValidatorClusterPoolMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{clusterAssignmentAnnotation: "missing"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:       v1alpha1.GPUDeviceStateReady,
			InventoryID: "inv-1",
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), device); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestGPUDeviceAssignmentValidatorClusterPoolSelectorMismatch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"inv-ok"}},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterPool).Build()
	validator := NewGPUDeviceAssignmentValidator(testr.New(t), cl)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-a",
			Annotations: map[string]string{clusterAssignmentAnnotation: "cluster-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-bad",
			State:       v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), device); err == nil || !strings.Contains(err.Error(), "does not match selector") {
		t.Fatalf("expected selector mismatch error, got %v", err)
	}
}
