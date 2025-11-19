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

func inventoryWithInfraReady(node string) *v1alpha1.GPUNodeInventory {
	return &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Conditions: []metav1.Condition{
				{Type: conditionGFDReady, Status: metav1.ConditionTrue},
				{Type: conditionDriverMissing, Status: metav1.ConditionFalse},
				{Type: conditionToolkitMissing, Status: metav1.ConditionFalse},
				{Type: conditionMonitoringMissing, Status: metav1.ConditionFalse},
			},
			Monitoring: v1alpha1.GPUNodeMonitoring{ConsecutiveHeartbeats: infraReadyHeartbeatThreshold},
			Bootstrap:  v1alpha1.GPUNodeBootstrapStatus{Phase: v1alpha1.GPUNodeBootstrapPhaseReady},
		},
	}
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
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateReady},
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

	inventory := inventoryWithInfraReady("node-a")

	for i := 0; i < 2; i++ {
		if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	}
	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected Ready, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerKeepsAssignedStates(t *testing.T) {
	states := []v1alpha1.GPUDeviceState{
		v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse,
		v1alpha1.GPUDeviceStatePendingAssignment,
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

func TestDeviceStateSyncHandlerMovesDiscoveredToValidatingWhenInfraReady(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateDiscovered},
	}
	client := newDeviceClient(t, device)
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := inventoryWithInfraReady("node-a")
	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateValidating {
		t.Fatalf("expected Validating, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerRecoversFaultedWhenInfraStable(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateFaulted},
	}
	client := newDeviceClient(t, device)
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := inventoryWithInfraReady("node-a")
	for i := 0; i < 2; i++ {
		if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
	}
	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected Ready, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerKeepsFaultedWhileHealthActive(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-a",
			State:    v1alpha1.GPUDeviceStateFaulted,
			Health: v1alpha1.GPUDeviceHealth{
				LastError:       "DCGM reported XID error",
				LastErrorReason: "XIDError",
			},
		},
	}
	client := newDeviceClient(t, device)
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := inventoryWithInfraReady("node-a")
	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateFaulted {
		t.Fatalf("expected Faulted state while health error present, got %s", updated.Status.State)
	}
}

func TestDeviceStateSyncHandlerFaultsReadyWhenDriverMissing(t *testing.T) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-dev"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-a", State: v1alpha1.GPUDeviceStateReady},
	}
	client := newDeviceClient(t, device)
	handler := NewDeviceStateSyncHandler(testr.New(t))
	handler.SetClient(client)

	inventory := inventoryWithInfraReady("node-a")
	for i := range inventory.Status.Conditions {
		if inventory.Status.Conditions[i].Type == conditionDriverMissing {
			inventory.Status.Conditions[i].Status = metav1.ConditionTrue
			break
		}
	}
	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-dev"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStateFaulted {
		t.Fatalf("expected Faulted, got %s", updated.Status.State)
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

	inventory := inventoryWithInfraReady("node-a")

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
	newDevice := func(state v1alpha1.GPUDeviceState) *v1alpha1.GPUDevice {
		return &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{State: state}}
	}
	withHealthFault := func() *v1alpha1.GPUDevice {
		return &v1alpha1.GPUDevice{
			Status: v1alpha1.GPUDeviceStatus{
				State: v1alpha1.GPUDeviceStateFaulted,
				Health: v1alpha1.GPUDeviceHealth{
					LastError: "XID",
				},
			},
		}
	}

	tests := []struct {
		name          string
		device        *v1alpha1.GPUDevice
		phase         v1alpha1.GPUNodeBootstrapPhase
		infraReady    bool
		infraDegraded bool
		want          v1alpha1.GPUDeviceState
		mutate        bool
	}{
		{"assigned-remains", newDevice(v1alpha1.GPUDeviceStateAssigned), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateAssigned, false},
		{"reserved-remains", newDevice(v1alpha1.GPUDeviceStateReserved), v1alpha1.GPUNodeBootstrapPhaseValidating, false, false, v1alpha1.GPUDeviceStateReserved, false},
		{"ready-stable", newDevice(v1alpha1.GPUDeviceStateReady), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateReady, false},
		{"ready-faulted-when-degraded", newDevice(v1alpha1.GPUDeviceStateReady), v1alpha1.GPUNodeBootstrapPhaseReady, true, true, v1alpha1.GPUDeviceStateFaulted, true},
		{"pending-faulted-when-degraded", newDevice(v1alpha1.GPUDeviceStatePendingAssignment), v1alpha1.GPUNodeBootstrapPhaseReady, true, true, v1alpha1.GPUDeviceStateFaulted, true},
		{"pending-stable-when-clean", newDevice(v1alpha1.GPUDeviceStatePendingAssignment), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStatePendingAssignment, false},
		{"discovered-to-validating-when-ready", newDevice(v1alpha1.GPUDeviceStateDiscovered), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateValidating, true},
		{"discovered-stays-when-not-ready", newDevice(v1alpha1.GPUDeviceStateDiscovered), v1alpha1.GPUNodeBootstrapPhaseValidating, false, false, v1alpha1.GPUDeviceStateDiscovered, false},
		{"faulted-health-blocks", withHealthFault(), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateFaulted, false},
		{"faulted-to-validating-when-ready", newDevice(v1alpha1.GPUDeviceStateFaulted), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateValidating, true},
		{"faulted-stays-when-not-ready", newDevice(v1alpha1.GPUDeviceStateFaulted), v1alpha1.GPUNodeBootstrapPhaseReady, false, false, v1alpha1.GPUDeviceStateFaulted, false},
		{"faulted-stays-when-degraded", newDevice(v1alpha1.GPUDeviceStateFaulted), v1alpha1.GPUNodeBootstrapPhaseReady, true, true, v1alpha1.GPUDeviceStateFaulted, false},
		{"validating-to-ready", newDevice(v1alpha1.GPUDeviceStateValidating), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateReady, true},
		{"validating-faulted-when-degraded", newDevice(v1alpha1.GPUDeviceStateValidating), v1alpha1.GPUNodeBootstrapPhaseReady, true, true, v1alpha1.GPUDeviceStateFaulted, true},
		{"validating-waits-when-not-ready", newDevice(v1alpha1.GPUDeviceStateValidating), v1alpha1.GPUNodeBootstrapPhaseReady, false, false, v1alpha1.GPUDeviceStateValidating, false},
		{"failure-phase-forces-fault", newDevice(v1alpha1.GPUDeviceStateReady), v1alpha1.GPUNodeBootstrapPhaseValidatingFailed, true, false, v1alpha1.GPUDeviceStateFaulted, true},
		{"failure-phase-keeps-discovered", newDevice(v1alpha1.GPUDeviceStateDiscovered), v1alpha1.GPUNodeBootstrapPhaseValidatingFailed, true, false, v1alpha1.GPUDeviceStateDiscovered, false},
		{"empty-state-promotes", newDevice(""), v1alpha1.GPUNodeBootstrapPhaseReady, true, false, v1alpha1.GPUDeviceStateValidating, true},
		{"empty-state-degraded", newDevice(""), v1alpha1.GPUNodeBootstrapPhaseReady, false, true, v1alpha1.GPUDeviceStateDiscovered, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, mutate := desiredDeviceState(tt.device, tt.phase, tt.infraReady, tt.infraDegraded)
			if got != tt.want || mutate != tt.mutate {
				t.Fatalf("expected (%s,%t), got (%s,%t)", tt.want, tt.mutate, got, mutate)
			}
		})
	}
}

func TestNormalizeDeviceState(t *testing.T) {
	if got := normalizeDeviceState(""); got != v1alpha1.GPUDeviceStateDiscovered {
		t.Fatalf("expected empty state to normalize to Discovered, got %s", got)
	}
	if got := normalizeDeviceState(v1alpha1.GPUDeviceStateReady); got != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected Ready to remain Ready, got %s", got)
	}
}
