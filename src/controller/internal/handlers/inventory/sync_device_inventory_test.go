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
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/go-logr/logr/testr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func newInventoryScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("register scheme: %v", err)
	}
	return scheme
}

type statusClient struct {
	client.Client
	status client.StatusWriter
}

func (c *statusClient) Status() client.StatusWriter {
	return c.status
}

type updatingStatusWriter struct {
	client.StatusWriter
	update func(context.Context, client.Object, ...client.SubResourceUpdateOption) error
}

func (w *updatingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if w.update != nil {
		return w.update(ctx, obj, opts...)
	}
	return w.StatusWriter.Update(ctx, obj, opts...)
}

type failingGetClient struct {
	client.Client
	err error
}

func (c *failingGetClient) Get(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
	return c.err
}

func TestDeviceInventorySyncSkipsWhenNodeNameMissing(t *testing.T) {
	scheme := newInventoryScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{}
	if res, err := h.HandleDevice(context.Background(), device); err != nil || res.Requeue {
		t.Fatalf("expected no-op for device without node name, got res=%+v err=%v", res, err)
	}
}

func TestDeviceInventorySyncName(t *testing.T) {
	scheme := newInventoryScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewDeviceInventorySync(testr.New(t), client)
	if h.Name() != "device-inventory-sync" {
		t.Fatalf("unexpected handler name: %s", h.Name())
	}
}

func TestDeviceInventorySyncInventoryNotFound(t *testing.T) {
	scheme := newInventoryScheme(t)
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "missing",
		},
	}
	if res, err := h.HandleDevice(context.Background(), device); err != nil || res.Requeue {
		t.Fatalf("expected no error for missing inventory, got res=%+v err=%v", res, err)
	}
}

func TestDeviceInventorySyncUpdatesExistingEntry(t *testing.T) {
	scheme := newInventoryScheme(t)
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices: []v1alpha1.GPUNodeDevice{{
				InventoryID: "node-a-0000",
				State:       v1alpha1.GPUDeviceStateDiscovered,
			}},
		},
	}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(inventory).
		Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "node-a-0000",
			State:       v1alpha1.GPUDeviceStateAssigned,
			AutoAttach:  true,
		},
	}

	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updated); err != nil {
		t.Fatalf("failed to fetch inventory: %v", err)
	}
	if len(updated.Status.Devices) != 1 {
		t.Fatalf("expected single device, got %d", len(updated.Status.Devices))
	}
	got := updated.Status.Devices[0]
	if got.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("device fields not updated: %+v", got)
	}
}

func TestDeviceInventorySyncAddsNewDevice(t *testing.T) {
	scheme := newInventoryScheme(t)
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(&v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}).
		Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "node-a-0001",
			State:       v1alpha1.GPUDeviceStateReserved,
			AutoAttach:  true,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "A100",
			},
		},
	}

	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updated); err != nil {
		t.Fatalf("failed to fetch inventory: %v", err)
	}
	if len(updated.Status.Devices) != 1 {
		t.Fatalf("expected device appended, got %d", len(updated.Status.Devices))
	}
	if updated.Status.Devices[0].Product != "A100" {
		t.Fatalf("expected hardware product propagated, got %+v", updated.Status.Devices[0])
	}
}

func TestDeviceInventorySyncRetriesOnConflict(t *testing.T) {
	scheme := newInventoryScheme(t)
	base := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(&v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}).
		Build()

	conflictErr := apierrors.NewConflict(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpunodeinventories"}, "node-a", errors.New("conflict"))
	writer := &updatingStatusWriter{
		StatusWriter: base.Status(),
		update: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
			return conflictErr
		},
	}
	client := &statusClient{Client: base, status: writer}
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{NodeName: "node-a"}}
	res, err := h.HandleDevice(context.Background(), device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Requeue {
		t.Fatal("expected requeue on conflict")
	}
}

func TestDeviceInventorySyncReturnsErrorOnGetFailure(t *testing.T) {
	client := &failingGetClient{err: errors.New("get failed")}
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{NodeName: "node-a"},
	}
	if _, err := h.HandleDevice(context.Background(), device); err == nil {
		t.Fatal("expected error on Get failure")
	}
}

func TestDeviceInventorySyncReturnsErrorOnUpdateFailure(t *testing.T) {
	scheme := newInventoryScheme(t)
	base := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(&v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}).
		Build()
	updateErr := errors.New("update failed")
	writer := &updatingStatusWriter{
		StatusWriter: base.Status(),
		update: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
			return updateErr
		},
	}
	client := &statusClient{Client: base, status: writer}
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "node-a-0002",
			State:       v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "A100",
				UUID:    "UUID-123",
			},
		},
	}

	if _, err := h.HandleDevice(context.Background(), device); err == nil {
		t.Fatal("expected update error to be returned")
	}
}

func TestDeviceInventorySyncUpdatesDevicesSliceOnly(t *testing.T) {
	scheme := newInventoryScheme(t)
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Devices: []v1alpha1.GPUNodeDevice{{
				InventoryID: "dev-1",
				State:       v1alpha1.GPUDeviceStateDiscovered,
			}},
		},
	}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(inventory).
		Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "dev-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updated); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if updated.Status.Devices[0].State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("state not updated: %+v", updated.Status.Devices[0])
	}
	if !updated.Status.Hardware.Present {
		t.Fatalf("hardware presence should reflect devices slice")
	}
}

func TestDeviceInventorySyncUpdatesHardwareSliceOnly(t *testing.T) {
	scheme := newInventoryScheme(t)
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: false},
		},
	}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(inventory).
		Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "dev-2",
			State:       v1alpha1.GPUDeviceStateAssigned,
		},
	}
	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updated); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if len(updated.Status.Devices) != 1 {
		t.Fatalf("devices slice should contain one entry: %+v", updated.Status.Devices)
	}
	if updated.Status.Devices[0].State != v1alpha1.GPUDeviceStateAssigned || !updated.Status.Hardware.Present {
		t.Fatalf("hardware/devices not updated: %+v", updated.Status.Hardware)
	}
}

func TestDeviceInventorySyncSetsDevicesSlice(t *testing.T) {
	scheme := newInventoryScheme(t)
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(&v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}).
		Build()
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "node-a-0003",
			State:       v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "A100",
				UUID:    "UUID-456",
			},
			Health: v1alpha1.GPUDeviceHealth{
				LastError:       "fault",
				LastErrorReason: "XID",
			},
		},
	}

	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.GPUNodeInventory{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node-a"}, updated); err != nil {
		t.Fatalf("fetch inventory: %v", err)
	}
	if len(updated.Status.Devices) != 1 {
		t.Fatalf("expected device recorded in status.devices, got %+v", updated.Status.Devices)
	}
	if !updated.Status.Hardware.Present {
		t.Fatalf("hardware presence not populated: %+v", updated.Status.Hardware)
	}
	if updated.Status.Devices[0].UUID != "UUID-456" || updated.Status.Devices[0].LastErrorReason != "XID" {
		t.Fatalf("device fields not propagated: %+v", updated.Status.Devices[0])
	}
}

func TestDeviceInventorySyncReturnsUpdateError(t *testing.T) {
	scheme := newInventoryScheme(t)
	base := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(&v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}).
		Build()

	writer := &updatingStatusWriter{
		StatusWriter: base.Status(),
		update: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
			return errors.New("boom")
		},
	}
	client := &statusClient{Client: base, status: writer}
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{NodeName: "node-a"}}
	if _, err := h.HandleDevice(context.Background(), device); err == nil {
		t.Fatal("expected update error to propagate")
	}
}

func TestDeviceInventorySyncReturnsGetError(t *testing.T) {
	scheme := newInventoryScheme(t)
	base := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		Build()
	client := &failingGetClient{Client: base, err: errors.New("get failed")}
	h := NewDeviceInventorySync(testr.New(t), client)

	device := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{NodeName: "node-a"}}
	if _, err := h.HandleDevice(context.Background(), device); err == nil {
		t.Fatal("expected get error to propagate")
	}
}
