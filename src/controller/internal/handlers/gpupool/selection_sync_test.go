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
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestSelectionSyncHandlerPicksDevicesAndCapacity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices: []v1alpha1.GPUNodeDevice{
				{InventoryID: "id-1", Product: "A100", State: v1alpha1.GPUDeviceStateReady},
				{InventoryID: "id-2", Product: "V100", State: v1alpha1.GPUDeviceStateReady},
				{InventoryID: "id-3", Product: "Ignore", State: v1alpha1.GPUDeviceStateFaulted},
			},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"node": "gpu"},
		},
	}

	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-2",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	dev3 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev3",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-3",
			State:       v1alpha1.GPUDeviceStateFaulted,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, node, dev1, dev2, dev3).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:              "Card",
				SlicesPerUnit:     2,
				MaxDevicesPerNode: func() *int32 { v := int32(1); return &v }(),
			},
			DeviceSelector: nil,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"node": "gpu"},
			},
		},
		Status: v1alpha1.GPUPoolStatus{},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}

	if len(pool.Status.Devices) != 3 {
		t.Fatalf("expected 3 devices (including faulted), got %d", len(pool.Status.Devices))
	}
	if pool.Status.Capacity.Total != 2 {
		t.Fatalf("expected capacity 2 (maxDevicesPerNode=1 with slicesPerUnit=2), got %d", pool.Status.Capacity.Total)
	}
	if pool.Status.Capacity.BaseUnits != 1 || pool.Status.Capacity.SlicesPerUnit != 2 {
		t.Fatalf("unexpected base units/slices: %+v", pool.Status.Capacity)
	}
	if len(pool.Status.Nodes) != 1 || pool.Status.Nodes[0].TotalDevices != 3 {
		t.Fatalf("unexpected node totals: %+v", pool.Status.Nodes)
	}
}

func TestSelectionSyncHandlerRespectsNodeSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices:  []v1alpha1.GPUNodeDevice{{InventoryID: "id-1", Product: "A100", State: v1alpha1.GPUDeviceStateReady}},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"node": "gpu"},
		},
	}

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, node, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource:       v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			NodeSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"node": "other"}},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	if len(pool.Status.Devices) != 0 || pool.Status.Capacity.Total != 0 {
		t.Fatalf("expected no devices due to nodeSelector, got %+v", pool.Status)
	}
}

func TestSelectionSyncUsesNodeLabelsWhenInventoryLabelMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices:  []v1alpha1.GPUNodeDevice{{InventoryID: "id-1", Product: "A100", State: v1alpha1.GPUDeviceStateReady}},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"node": "gpu"},
		},
	}
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, node, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource:       v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			NodeSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"node": "gpu"}},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	if len(pool.Status.Devices) != 1 || pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected devices matched by node labels, got %+v", pool.Status)
	}
}

func TestSelectionSyncFallsBackToInventoryLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"role": "gpu"},
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices:  []v1alpha1.GPUNodeDevice{{InventoryID: "id-1", State: v1alpha1.GPUDeviceStateReady}},
		},
	}
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource:       v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			NodeSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"role": "gpu"}},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	if len(pool.Status.Devices) != 1 || pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected device matched via inventory labels, got %+v", pool.Status)
	}
}

func TestSelectionSyncSkipsUnassignedDevicesInPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices:  []v1alpha1.GPUNodeDevice{{InventoryID: "id-1", State: v1alpha1.GPUDeviceStateReady}},
		},
	}
	// device exists but assigned to another pool
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "other"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	if len(pool.Status.Devices) != 0 || pool.Status.Capacity.Total != 0 {
		t.Fatalf("expected unassigned devices skipped, got %+v", pool.Status)
	}
}

func TestSelectionSyncHandlerMIGCapacity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices: []v1alpha1.GPUNodeDevice{
				{
					InventoryID: "id-1",
					Product:     "A100",
					State:       v1alpha1.GPUDeviceStateReady,
					MIG: v1alpha1.GPUMIGConfig{
						Capable: true,
						Types: []v1alpha1.GPUMIGTypeCapacity{
							{Name: "1g.10gb", Count: 2},
						},
					},
				},
			},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"node": "gpu"},
		},
	}

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, node, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "MIG",
				MIGProfile:    "1g.10gb",
				SlicesPerUnit: 2,
			},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	if pool.Status.Capacity.Total != 4 {
		t.Fatalf("expected MIG capacity 4 (2 partitions * slices 2), got %d", pool.Status.Capacity.Total)
	}
	if pool.Status.Capacity.BaseUnits != 2 || pool.Status.Capacity.SlicesPerUnit != 2 {
		t.Fatalf("unexpected capacity details: %+v", pool.Status.Capacity)
	}
}

func TestSelectionSyncHandlerMaxDevicesPerNodeZeroReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"node": "gpu"},
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices: []v1alpha1.GPUNodeDevice{
				{InventoryID: "id-1", Product: "A100", State: v1alpha1.GPUDeviceStateFaulted},
				{InventoryID: "id-2", Product: "A100", State: v1alpha1.GPUDeviceStateFaulted},
			},
		},
	}

	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-1",
			State:       v1alpha1.GPUDeviceStateFaulted,
		},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id-2",
			State:       v1alpha1.GPUDeviceStateFaulted,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev1, dev2).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	if pool.Status.Capacity.Total != 0 {
		t.Fatalf("expected zero capacity when no ready devices, got %d", pool.Status.Capacity.Total)
	}
	if pool.Status.Capacity.BaseUnits != 0 {
		t.Fatalf("expected zero base units, got %d", pool.Status.Capacity.BaseUnits)
	}
	if len(pool.Status.Devices) != 2 || pool.Status.Devices[0].State != v1alpha1.GPUDeviceStateFaulted {
		t.Fatalf("expected devices listed with faulted state, got %+v", pool.Status.Devices)
	}
}
