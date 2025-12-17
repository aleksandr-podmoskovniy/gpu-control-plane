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

package inventory

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type assignmentListErrorClient struct {
	client.Client
	err error
}

func (c *assignmentListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.err
}

func TestGPUDeviceAssignmentValidatorClusterPoolNotFoundAndMismatch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-a",
			Annotations: map[string]string{clusterAssignmentAnnotation: "missing"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State: v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-uuid-1",
				PCI:  v1alpha1.PCIAddress{Address: "0000:01:00.0"},
			},
		},
	}
	if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
		t.Fatalf("expected denial for missing cluster pool")
	}

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
	validator = NewGPUDeviceAssignmentValidator(testr.New(t), cl)

	device.Annotations[clusterAssignmentAnnotation] = "cluster-a"
	device.Status.InventoryID = "inv-2"
	if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
		t.Fatalf("expected denial for selector mismatch")
	}
}

func TestMatchesDeviceSelectorBranches(t *testing.T) {
	if matchesDeviceSelector(nil, nil) {
		t.Fatalf("expected nil device to not match")
	}

	dev := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{InventoryID: "inv"}}
	if !matchesDeviceSelector(dev, nil) {
		t.Fatalf("expected nil selector to match")
	}
}

func TestResolveNamespacedPoolByNameErrors(t *testing.T) {
	ctx := context.Background()

	if _, err := resolveNamespacedPoolByName(ctx, nil, " "); err == nil {
		t.Fatalf("expected error for empty name")
	}
	if _, err := resolveNamespacedPoolByName(ctx, nil, "pool"); err == nil {
		t.Fatalf("expected error for nil client")
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	badList := &assignmentListErrorClient{Client: base, err: apierrors.NewBadRequest("list failed")}
	if _, err := resolveNamespacedPoolByName(ctx, badList, "pool"); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected list error, got %v", err)
	}
}
