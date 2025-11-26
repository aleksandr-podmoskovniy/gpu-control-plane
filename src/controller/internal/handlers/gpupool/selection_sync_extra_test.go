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

func TestSelectionSyncHandlesInvalidNodeSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	handler := NewSelectionSyncHandler(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
	selector := metav1.LabelSelector{MatchLabels: map[string]string{"bad key": "v"}}
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{NodeSelector: &selector}}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected error for invalid node selector")
	}
}

func TestSelectionSyncHandlesConflicts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	// client returns conflict on list to exercise conflict branch
	cl := &failingClient{err: apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("nodes").GroupResource(), "pool", nil)}
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestSelectionSyncHandlesDeviceListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	inv := &v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv).Build()
	cl := &selectiveFailClient{Client: base, failDevices: true}
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected device list error")
	}
}

type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return f.err
}

type selectiveFailClient struct {
	client.Client
	failDevices bool
}

func (f *selectiveFailClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.failDevices {
		if _, ok := list.(*v1alpha1.GPUDeviceList); ok {
			return apierrors.NewBadRequest("fail devices")
		}
	}
	return f.Client.List(ctx, list, opts...)
}

func TestSelectionSyncHandlePoolHappyPath(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{
					{InventoryID: "dev1"},
					{InventoryID: "dev2", MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "1g.10gb", Count: 1}}}},
				},
			},
		},
	}
	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{"gpu.deckhouse.io/assignment": "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateReady},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{"gpu.deckhouse.io/assignment": "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev2", State: v1alpha1.GPUDeviceStateReady},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev1, dev2).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:          "MIG",
				MIGProfile:    "1g.10gb",
				SlicesPerUnit: 2,
			},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Exclude: v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"devX"}},
			},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool failed: %v", err)
	}
	if pool.Status.Capacity.Total != 2 {
		t.Fatalf("expected capacity 2, got %d", pool.Status.Capacity.Total)
	}
	if len(pool.Status.Nodes) != 1 {
		t.Fatalf("expected one node, got %d", len(pool.Status.Nodes))
	}
}

func TestSelectionSyncSkipsUnassignedDevices(t *testing.T) {
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
	// device assigned to another pool
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev1", Annotations: map[string]string{assignmentAnnotation: "other"}},
		Status:     v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateReady},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv, dev).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 0 || len(pool.Status.Devices) != 0 {
		t.Fatalf("unassigned device should be ignored, got %+v", pool.Status)
	}
}

func TestCleanupPoolResourcesError(t *testing.T) {
	// client that returns conflict on delete to hit error branch
	cl := &failingDeleteClient{deleteErr: apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("daemonsets").GroupResource(), "ds", nil)}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err == nil {
		t.Fatalf("expected error from cleanupPoolResources")
	}
}

func TestCleanupPoolResourcesSuccess(t *testing.T) {
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
		t.Fatalf("cleanupPoolResources failed: %v", err)
	}
	// ensure objects are gone
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "nvidia-device-plugin-pool", Namespace: "ns"}, &appsv1.DaemonSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected DS deleted, got %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "nvidia-device-plugin-pool-config", Namespace: "ns"}, &corev1.ConfigMap{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected CM deleted, got %v", err)
	}
}

func TestCleanupPoolResourcesNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupPoolResources(context.Background(), "pool"); err != nil {
		t.Fatalf("expected cleanup without objects to succeed, got %v", err)
	}
}

func TestCleanupMIGResourcesError(t *testing.T) {
	cl := &failingDeleteClient{deleteErr: apierrors.NewBadRequest("boom")}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupMIGResources(context.Background(), "pool"); err == nil {
		t.Fatalf("expected error from cleanupMIGResources")
	}
}

func TestCleanupMIGResourcesSuccess(t *testing.T) {
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
		t.Fatalf("cleanupMIGResources failed: %v", err)
	}
	for _, name := range []string{
		"nvidia-mig-manager-pool",
		"nvidia-mig-manager-pool-config",
		"nvidia-mig-manager-pool-scripts",
		"nvidia-mig-manager-pool-gpu-clients",
	} {
		if err := cl.Get(context.Background(), client.ObjectKey{Name: name, Namespace: "ns"}, &corev1.ConfigMap{}); err == nil {
			t.Fatalf("%s should be deleted", name)
		}
	}
}

func TestCleanupMIGResourcesNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns"})
	if err := h.cleanupMIGResources(context.Background(), "pool"); err != nil {
		t.Fatalf("expected cleanup to ignore notfound, got %v", err)
	}
}

func TestUnitsForDeviceCardDefault(t *testing.T) {
	h := NewSelectionSyncHandler(testr.New(t), nil)
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 0}}}
	if units, base := h.unitsForDevice(v1alpha1.GPUNodeDevice{}, pool); units != 1 || base != 1 {
		t.Fatalf("expected default units/base 1/1, got %d/%d", units, base)
	}
}

func TestUnitsForDeviceMIGMissingProfile(t *testing.T) {
	h := NewSelectionSyncHandler(testr.New(t), nil)
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: ""}}}
	if units, base := h.unitsForDevice(v1alpha1.GPUNodeDevice{}, pool); units != 0 || base != 0 {
		t.Fatalf("expected zero units for missing mig profile, got %d/%d", units, base)
	}

	pool.Spec.Resource.MIGProfile = "1g.10gb"
	dev := v1alpha1.GPUNodeDevice{MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "2g.20gb", Count: 1}}}}
	if units, base := h.unitsForDevice(dev, pool); units != 0 || base != 0 {
		t.Fatalf("expected zero when device lacks profile, got %d/%d", units, base)
	}

	dev.MIG.Types = []v1alpha1.GPUMIGTypeCapacity{{Name: "1g.10gb", Count: 1}}
	pool.Spec.Resource.SlicesPerUnit = 0
	if units, base := h.unitsForDevice(dev, pool); units != 1 || base != 1 {
		t.Fatalf("expected slices fallback to profile count, got %d/%d", units, base)
	}

	pool.Spec.Resource.SlicesPerUnit = 3
	if units, base := h.unitsForDevice(dev, pool); units != 3 || base != 1 {
		t.Fatalf("expected slices override, got %d/%d", units, base)
	}

	pool.Spec.Resource.Unit = "Card"
	pool.Spec.Resource.SlicesPerUnit = 5
	if units, base := h.unitsForDevice(dev, pool); units != 5 || base != 1 {
		t.Fatalf("expected card slices override, got %d/%d", units, base)
	}
}

type failingDeleteClient struct {
	client.Client
	deleteErr error
}

func (f *failingDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return f.deleteErr
}

func TestHandlePoolMultipleNodesAndFilters(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	inv1 := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"env": "prod"}},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{
					{InventoryID: "dev1"},
					{InventoryID: "dev2"},
				},
			},
		},
	}
	inv2 := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-b", Labels: map[string]string{"env": "dev"}},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{
					{InventoryID: "dev3"},
				},
			},
		},
	}

	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateReady},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
			Labels:      map[string]string{"gpu.deckhouse.io/ignore": "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev2", State: v1alpha1.GPUDeviceStateReady},
	}
	dev3 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev3",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev3", State: v1alpha1.GPUDeviceStateFaulted},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inv1, inv2, dev1, dev2, dev3).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)

	max := int32(1)
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			NodeSelector: selector,
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:              "Card",
				SlicesPerUnit:     1,
				MaxDevicesPerNode: &max,
			},
		},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool failed: %v", err)
	}
	if pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected capacity 1 (max per node), got %d", pool.Status.Capacity.Total)
	}
	if len(pool.Status.Devices) != 1 {
		t.Fatalf("expected only ready non-ignored device counted, got %d", len(pool.Status.Devices))
	}
}
