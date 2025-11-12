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

package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func newDeviceClient(t *testing.T, objs ...runtime.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithIndex(&v1alpha1.GPUDevice{}, deviceNodeIndexKey, func(obj client.Object) []string {
			dev := obj.(*v1alpha1.GPUDevice)
			if dev.Status.NodeName == "" {
				return nil
			}
			return []string{dev.Status.NodeName}
		})
	if len(objs) > 0 {
		builder = builder.WithRuntimeObjects(objs...)
	}
	return builder.Build()
}

type failingDeviceListClient struct {
	client.Client
	err error
}

func (f *failingDeviceListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return f.err
}

type failingStatusWriter struct{}

func (f failingStatusWriter) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return nil
}

func (f failingStatusWriter) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return nil
}

func (f failingStatusWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return errors.New("patch boom")
}

type failingDevicePatchClient struct {
	client.Client
}

func (f *failingDevicePatchClient) Status() client.StatusWriter {
	return failingStatusWriter{}
}

func TestDeviceStateSyncHandlerKeepsDevicesDiscoveredWhileValidating(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateDiscovered},
	}

	client := newDeviceClient(t, device)

	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionFalse}},
			Bootstrap:  v1alpha1.GPUNodeBootstrapStatus{Phase: v1alpha1.GPUNodeBootstrapPhaseValidating},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateDiscovered {
		t.Fatalf("expected device state Discovered, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerMarksDevicesFaultedOnFailure(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateReadyForPooling},
	}

	client := newDeviceClient(t, device)

	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionFalse}},
			Bootstrap:  v1alpha1.GPUNodeBootstrapStatus{Phase: v1alpha1.GPUNodeBootstrapPhaseValidatingFailed},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateFaulted {
		t.Fatalf("expected device state Faulted, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerDoesNotFaultNewDevices(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateDiscovered},
	}

	client := newDeviceClient(t, device)

	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionFalse}},
			Bootstrap:  v1alpha1.GPUNodeBootstrapStatus{Phase: v1alpha1.GPUNodeBootstrapPhaseValidatingFailed},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateDiscovered {
		t.Fatalf("expected device state Discovered, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerNameAndClientValidation(t *testing.T) {
	handler := NewDeviceStateSyncHandler(testr.New(t))
	if handler.Name() != "device-state-sync" {
		t.Fatalf("unexpected handler name: %s", handler.Name())
	}
	inventory := &v1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	if _, err := handler.HandleNode(context.Background(), inventory); err == nil {
		t.Fatal("expected error when client is not configured")
	}
}

func TestDeviceStateSyncHandlerSkipsEmptyNode(t *testing.T) {
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(newDeviceClient(t))
	if _, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeInventory{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeviceStateSyncHandlerPromotesToReady(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateDiscovered},
	}
	client := newDeviceClient(t, device)

	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionTrue}},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateReadyForPooling {
		t.Fatalf("expected ReadyForPooling, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerKeepsAssignedStates(t *testing.T) {
	states := []v1alpha1.GPUDeviceState{
		v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse,
		v1alpha1.GPUDeviceStatePendingAssignment,
		v1alpha1.GPUDeviceStateNoPoolMatched,
	}
	for _, state := range states {
		device := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: string(state)},
			Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: state},
		}
		client := newDeviceClient(t, device)
		handler := NewDeviceStateSyncHandler(testr.New(t))
		handler.SetClient(client)

		inventory := &v1alpha1.GPUNodeInventory{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
			Status: v1alpha1.GPUNodeInventoryStatus{
				Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionTrue}},
			},
		}

		if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		updated := &v1alpha1.GPUDevice{}
		if err := client.Get(context.Background(), types.NamespacedName{Name: string(state)}, updated); err != nil {
			t.Fatalf("get: %v", err)
		}
		if updated.Status.State != state {
			t.Fatalf("expected state %s to remain unchanged, got %s", state, updated.Status.State)
		}
	}
}

func TestDeviceStateSyncHandlerNoDevicesOnNode(t *testing.T) {
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(newDeviceClient(t))
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     v1alpha1.GPUNodeInventoryStatus{},
	}
	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeviceStateSyncHandlerHandlesListError(t *testing.T) {
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(&failingDeviceListClient{err: errors.New("boom")})

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionTrue}},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err == nil {
		t.Fatal("expected list error")
	}
}

func TestDeviceStateSyncHandlerAggregatesPatchErrors(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateDiscovered},
	}
	baseClient := newDeviceClient(t, device)

	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(&failingDevicePatchClient{Client: baseClient})

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: conditionReadyForPooling, Status: metav1.ConditionTrue}},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err == nil || !strings.Contains(err.Error(), "patch device") {
		t.Fatalf("expected aggregated patch error, got %v", err)
	}
}

func TestIsConditionTrue(t *testing.T) {
	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{{Type: "A", Status: metav1.ConditionTrue}},
		},
	}
	if !isConditionTrue(inventory, "A") {
		t.Fatal("expected condition A to be true")
	}
	if isConditionTrue(inventory, "B") {
		t.Fatal("expected missing condition to be false")
	}
}

func TestDesiredDeviceState(t *testing.T) {
	tests := []struct {
		name    string
		current v1alpha1.GPUDeviceState
		ready   bool
		phase   v1alpha1.GPUNodeBootstrapPhase
		want    v1alpha1.GPUDeviceState
		mutate  bool
	}{
		{"assigned", v1alpha1.GPUDeviceStateAssigned, true, v1alpha1.GPUNodeBootstrapPhaseReady, v1alpha1.GPUDeviceStateAssigned, false},
		{"reserved", v1alpha1.GPUDeviceStateReserved, false, v1alpha1.GPUNodeBootstrapPhaseValidating, v1alpha1.GPUDeviceStateReserved, false},
		{"ready-stays", v1alpha1.GPUDeviceStateReadyForPooling, true, v1alpha1.GPUNodeBootstrapPhaseReady, v1alpha1.GPUDeviceStateReadyForPooling, false},
		{"ready-pending", v1alpha1.GPUDeviceStateReadyForPooling, false, v1alpha1.GPUNodeBootstrapPhaseGFD, v1alpha1.GPUDeviceStateDiscovered, true},
		{"ready-faults", v1alpha1.GPUDeviceStateReadyForPooling, false, v1alpha1.GPUNodeBootstrapPhaseValidatingFailed, v1alpha1.GPUDeviceStateFaulted, true},
		{"discovered-ready", v1alpha1.GPUDeviceStateDiscovered, true, v1alpha1.GPUNodeBootstrapPhaseReady, v1alpha1.GPUDeviceStateReadyForPooling, true},
		{"discovered-pending", v1alpha1.GPUDeviceStateDiscovered, false, v1alpha1.GPUNodeBootstrapPhaseValidating, v1alpha1.GPUDeviceStateDiscovered, false},
		{"discovered-failed-stays", v1alpha1.GPUDeviceStateDiscovered, false, v1alpha1.GPUNodeBootstrapPhaseValidatingFailed, v1alpha1.GPUDeviceStateDiscovered, false},
		{"faulted-holds-on-failure", v1alpha1.GPUDeviceStateFaulted, false, v1alpha1.GPUNodeBootstrapPhaseValidatingFailed, v1alpha1.GPUDeviceStateFaulted, false},
		{"faulted-to-ready", v1alpha1.GPUDeviceStateFaulted, true, v1alpha1.GPUNodeBootstrapPhaseReady, v1alpha1.GPUDeviceStateReadyForPooling, true},
		{"faulted-to-discovered", v1alpha1.GPUDeviceStateFaulted, false, v1alpha1.GPUNodeBootstrapPhaseMonitoring, v1alpha1.GPUDeviceStateDiscovered, true},
		{"legacy-unassigned", v1alpha1.GPUDeviceStateUnassigned, false, v1alpha1.GPUNodeBootstrapPhaseValidating, v1alpha1.GPUDeviceStateDiscovered, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, mutate := desiredDeviceState(tt.current, tt.ready, tt.phase)
			if got != tt.want || mutate != tt.mutate {
				t.Fatalf("expected (%s,%t), got (%s,%t)", tt.want, tt.mutate, got, mutate)
			}
		})
	}
}
