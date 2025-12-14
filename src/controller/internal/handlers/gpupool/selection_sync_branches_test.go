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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

type gpuDeviceListCallErrorClient struct {
	client.Client
	err        error
	callNumber int
	calls      int
}

func (c *gpuDeviceListCallErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v1alpha1.GPUDeviceList); ok {
		c.calls++
		if c.calls == c.callNumber {
			return c.err
		}
	}
	return c.Client.List(ctx, list, opts...)
}

type fixedFirstDeviceListClient struct {
	client.Client
	items []v1alpha1.GPUDevice
}

func (c *fixedFirstDeviceListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if out, ok := list.(*v1alpha1.GPUDeviceList); ok {
		out.Items = append([]v1alpha1.GPUDevice(nil), c.items...)
		return nil
	}
	return c.Client.List(ctx, list, opts...)
}

func TestSelectionSyncClusterPoolUsesClusterAssignmentField(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{clusterAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id",
			State:       v1alpha1.GPUDeviceStateReserved,
			NodeName:    "node1",
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterGPUPool"},
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total == 0 {
		t.Fatalf("expected capacity computed for cluster pool")
	}
}

func TestSelectionSyncPropagatesPoolRefDevicesListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	listErr := apierrors.NewBadRequest("poolRef list failed")
	cl := &gpuDeviceListCallErrorClient{Client: base, err: listErr, callNumber: 2}

	h := NewSelectionSyncHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected poolRefDevices list error, got %v", err)
	}
}

func TestSelectionSyncSkipsInvalidAssignmentsAndIgnoresDevicesWithoutNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// First list call (assignedDevices) is overridden to cover unreachable/defensive branches.
	items := []v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "mismatch", Annotations: map[string]string{assignmentAnnotation: "other"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ignored", Annotations: map[string]string{assignmentAnnotation: "pool"}, Labels: map[string]string{deviceIgnoreKey: "true"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nonode", Annotations: map[string]string{assignmentAnnotation: "pool"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "fallback", Annotations: map[string]string{assignmentAnnotation: "pool"}, Labels: map[string]string{"kubernetes.io/hostname": "node1"}},
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "",
				State:       v1alpha1.GPUDeviceStateReserved,
				PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "named", Annotations: map[string]string{assignmentAnnotation: "pool"}},
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "id",
				NodeName:    "node1",
				State:       v1alpha1.GPUDeviceStateReserved,
				PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
			},
		},
	}

	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(&items[3], &items[4]).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), &fixedFirstDeviceListClient{Client: base, items: items})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 2 {
		t.Fatalf("expected capacity from two reserved devices, got %+v", pool.Status.Capacity)
	}
}

func TestSelectionSyncNodeSelectorSkipsMissingNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id",
			State:       v1alpha1.GPUDeviceStateReserved,
			NodeName:    "missing-node",
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource:     v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "gpu"}},
		},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 0 {
		t.Fatalf("expected missing nodes to be filtered out, got %+v", pool.Status.Capacity)
	}
}

func TestSelectionSyncMaxDevicesPerNodeAndUnitsZero(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	max := int32(1)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", MaxDevicesPerNode: &max},
		},
	}

	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Annotations: map[string]string{assignmentAnnotation: "pool"}},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateReserved,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Annotations: map[string]string{assignmentAnnotation: "pool"}},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateReserved,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev1, dev2).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected maxDevicesPerNode limit, got %+v", pool.Status.Capacity)
	}

	// Units <= 0 branch for MIG pools without profile.
	poolMIG := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "mig", Namespace: "ns"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"},
		},
	}
	devMIG := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "mig-dev", Annotations: map[string]string{assignmentAnnotation: "mig"}},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "mig",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateReserved,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "mig"},
		},
	}
	cl = withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(devMIG).
		Build()
	h = NewSelectionSyncHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), poolMIG); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	if poolMIG.Status.Capacity.Total != 0 {
		t.Fatalf("expected zero capacity for MIG without profile, got %+v", poolMIG.Status.Capacity)
	}
}

func TestSelectionSyncUnassignSkipsAnnotatedAndNamespaceMismatchedRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	keep := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "keep",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateReserved,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	mismatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "mismatch",
			Annotations: map[string]string{},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateReserved,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(keep, mismatch).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
}

func TestNeedsAssignmentUpdateNamespaceMismatchBranches(t *testing.T) {
	dev := v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"}, State: v1alpha1.GPUDeviceStateReserved}}
	if !needsAssignmentUpdate(dev, "pool", "") {
		t.Fatalf("expected namespaced poolRef to require update for cluster pool")
	}
	dev.Status.PoolRef.Namespace = "other"
	if !needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected pool namespace mismatch to require update")
	}
}

func TestClearDevicePoolNamespaceMismatchBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.GPUDevice{}).WithObjects(dev).Build()
	h := NewSelectionSyncHandler(testr.New(t), cl)
	if err := h.clearDevicePool(context.Background(), "dev", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected namespace mismatch to short-circuit: %v", err)
	}
	loaded := &v1alpha1.GPUDevice{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "dev"}, loaded)
	if loaded.Status.PoolRef == nil {
		t.Fatalf("expected poolRef to remain set")
	}

	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev2"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"},
		},
	}
	cl = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.GPUDevice{}).WithObjects(dev2).Build()
	h = NewSelectionSyncHandler(testr.New(t), cl)
	if err := h.clearDevicePool(context.Background(), "dev2", "pool", "ns", assignmentAnnotation); err != nil {
		t.Fatalf("expected different namespace to short-circuit: %v", err)
	}
}
