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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestSelectionSyncName(t *testing.T) {
	h := NewSelectionSyncHandler(testr.New(t), nil)
	if h.Name() != "selection-sync" {
		t.Fatalf("unexpected name %s", h.Name())
	}
}

func TestHandlePoolStatesAndAutoAttach(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "dev-ready"}, {InventoryID: "dev-assigned"}, {InventoryID: "dev-auto"}},
			},
		},
	}
	devReady := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-ready",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev-ready", State: v1alpha1.GPUDeviceStateReady},
	}
	devAssigned := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-assigned",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev-assigned", State: v1alpha1.GPUDeviceStateAssigned},
	}
	devAuto := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-auto",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev-auto", State: v1alpha1.GPUDeviceStateReady, AutoAttach: true},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, devReady, devAssigned, devAuto).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if len(pool.Status.Devices) != 3 {
		t.Fatalf("expected 3 devices in status, got %d", len(pool.Status.Devices))
	}
	// only ready devices contribute to capacity
	if pool.Status.Capacity.Total != 2 {
		t.Fatalf("expected capacity 2 (ready+auto), got %d", pool.Status.Capacity.Total)
	}
}

func TestHandlePoolMaxDevicesPerNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "dev1"}, {InventoryID: "dev2"}},
			},
		},
	}
	dev1 := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev1", Annotations: map[string]string{assignmentAnnotation: "pool"}}, Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateReady}}
	dev2 := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev2", Annotations: map[string]string{assignmentAnnotation: "pool"}}, Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev2", State: v1alpha1.GPUDeviceStateReady}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev1, dev2).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	max := int32(1)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1, MaxDevicesPerNode: &max},
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected maxDevicesPerNode to cap capacity to 1, got %d", pool.Status.Capacity.Total)
	}
}

func TestHandlePoolUsedExceedsTotal(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "dev1"}}},
		},
	}
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev1", Annotations: map[string]string{assignmentAnnotation: "pool"}},
		Status:     v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateFaulted},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev).Build()
	h := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Used: 2}},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Available != 0 {
		t.Fatalf("available should remain zero when used>total, got %d", pool.Status.Capacity.Available)
	}
}

func TestHandlePoolNodeSelectorExcludesAll(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
		},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "dev1"}},
			},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"role": "gpu"},
		},
	}
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateReady},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, node, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "cpu"}},
			Resource:     v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 0 {
		t.Fatalf("expected zero capacity when no nodes match selector, got %d", pool.Status.Capacity.Total)
	}
	if len(pool.Status.Devices) != 0 {
		t.Fatalf("expected no devices recorded, got %d", len(pool.Status.Devices))
	}
}

func TestCleanupPoolResourcesDeletesAllKinds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	objs := []client.Object{
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool", Namespace: "ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool-config", Namespace: "ns"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err != nil {
		t.Fatalf("cleanupPoolResources: %v", err)
	}
}

func TestCleanupPoolResourcesPropagatesMIGError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	errs := map[string]error{
		"nvidia-mig-manager-pool": apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("daemonsets").GroupResource(), "ds", nil),
	}
	custom := &errorDeleteClient{Client: cl, errs: errs}
	h := NewRendererHandler(testr.New(t), custom, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err == nil {
		t.Fatalf("expected error propagated from MIG cleanup")
	}
}

type errorDeleteClient struct {
	client.Client
	errs map[string]error
}

func (c *errorDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err, ok := c.errs[obj.GetName()]; ok {
		return err
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestCleanupMIGResourcesDeletesAllKinds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	objs := []client.Object{
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-pool", Namespace: "ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-pool-config", Namespace: "ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-pool-scripts", Namespace: "ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-mig-manager-pool-gpu-clients", Namespace: "ns"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupMIGResources(context.Background(), "pool"); err != nil {
		t.Fatalf("cleanupMIGResources: %v", err)
	}
}

func TestUnitsForDeviceCardZeroSlices(t *testing.T) {
	h := NewSelectionSyncHandler(testr.New(t), nil)
	pool := &v1alpha1.GPUPool{
		Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 0}},
	}
	if units, base := h.unitsForDevice(v1alpha1.GPUNodeDevice{}, pool); units != 1 || base != 1 {
		t.Fatalf("expected default 1/1, got %d/%d", units, base)
	}
}

func TestHandlePoolIgnoresMarkedDevices(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "dev1"}},
			},
		},
	}
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
			Labels:      map[string]string{"gpu.deckhouse.io/ignore": "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateReady},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev).Build()
	h := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 0 || len(pool.Status.Devices) != 0 {
		t.Fatalf("ignored device should not contribute to capacity, status=%+v", pool.Status)
	}
}
