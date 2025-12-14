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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

type listFailClient struct {
	client.Client
	err error
}

func (c *listFailClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

func newWorkloadClient(t *testing.T, objs ...runtime.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		builder = builder.WithRuntimeObjects(objs...)
	}
	return builder.Build()
}

func TestWorkloadStatusHandlerMarksReadyWhenAllChecksPassed(t *testing.T) {
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "dev-1",
			Labels: map[string]string{gpuDeviceNodeLabelKey: "node-1"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t, dev))

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1alpha1.GPUNodeStateStatus{
			Conditions: []metav1.Condition{{
				Type:   conditionInventoryComplete,
				Status: metav1.ConditionTrue,
			}},
		},
	}

	ctx := validation.ContextWithStatus(context.Background(), validation.Result{
		DriverReady:       true,
		ToolkitReady:      true,
		DCGMReady:         true,
		DCGMExporterReady: true,
		MonitoringReady:   true,
		GFDReady:          true,
		Ready:             true,
	})

	res, err := handler.HandleNode(ctx, inventory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter != defaultReadyRequeueDelay {
		t.Fatalf("expected ready requeue delay, got %s", res.RequeueAfter)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionReadyForPooling); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected ReadyForPooling condition true, got %v", cond)
	}
}

func TestWorkloadStatusHandlerTracksPendingValidationAndAttempts(t *testing.T) {
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "dev-2",
			Labels: map[string]string{gpuDeviceNodeLabelKey: "node-2"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-2",
			State:       v1alpha1.GPUDeviceStateDiscovered,
		},
	}

	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t, dev))

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status: v1alpha1.GPUNodeStateStatus{
			Conditions: []metav1.Condition{{
				Type:   conditionInventoryComplete,
				Status: metav1.ConditionTrue,
			}},
		},
	}

	ctx := validation.ContextWithStatus(context.Background(), validation.Result{
		DriverReady:       true,
		ToolkitReady:      true,
		DCGMReady:         true,
		DCGMExporterReady: true,
		MonitoringReady:   true,
		GFDReady:          true,
		Ready:             true,
	})

	res, err := handler.HandleNode(ctx, inventory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter != defaultNotReadyRequeueDelay {
		t.Fatalf("expected not-ready requeue delay, got %s", res.RequeueAfter)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionReadyForPooling); cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonDevicesPending {
		t.Fatalf("expected ReadyForPooling=false with reason %s, got %v", reasonDevicesPending, cond)
	}
}

func getCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func TestWorkloadStatusHandlerNameAndManagedDisabledHelper(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t))
	if handler.Name() == "" {
		t.Fatalf("expected handler name")
	}
}

func TestWorkloadStatusHandlerEarlyReturnsAndListErrors(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t))

	t.Run("empty node name returns nil", func(t *testing.T) {
		res, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeState{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 0 {
			t.Fatalf("expected empty result, got %+v", res)
		}
	})

	t.Run("nil client returns error", func(t *testing.T) {
		_, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}})
		if err == nil {
			t.Fatalf("expected error when client is not configured")
		}
	})

	t.Run("list error bubbles up", func(t *testing.T) {
		base := newWorkloadClient(t)
		handler.SetClient(&listFailClient{Client: base, err: errors.New("list failed")})

		_, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}})
		if err == nil || !strings.Contains(err.Error(), "list GPUDevices") {
			t.Fatalf("expected list GPUDevices error, got %v", err)
		}
	})
}

func TestWorkloadStatusHandlerWithoutValidatorStatusUsesWorkloadReadiness(t *testing.T) {
	t.Run("infra degraded with running workloads", func(t *testing.T) {
		dev := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "dev-1",
				Labels: map[string]string{gpuDeviceNodeLabelKey: "node-1"},
			},
			Status: v1alpha1.GPUDeviceStatus{
				State: v1alpha1.GPUDeviceStateAssigned,
			},
		}

		handler := NewWorkloadStatusHandler(testr.New(t))
		handler.SetClient(newWorkloadClient(t, dev))

		inventory := &v1alpha1.GPUNodeState{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: v1alpha1.GPUNodeStateStatus{
				Conditions: []metav1.Condition{{
					Type:   conditionInventoryComplete,
					Status: metav1.ConditionTrue,
				}},
			},
		}

		res, err := handler.HandleNode(context.Background(), inventory)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != defaultNotReadyRequeueDelay {
			t.Fatalf("expected not-ready requeue delay, got %s", res.RequeueAfter)
		}
		if cond := getCondition(inventory.Status.Conditions, conditionInfraDegraded); cond == nil || cond.Status != metav1.ConditionTrue {
			t.Fatalf("expected InfraDegraded condition true, got %v", cond)
		}
		if cond := getCondition(inventory.Status.Conditions, conditionDegradedWorkloads); cond == nil || cond.Status != metav1.ConditionTrue {
			t.Fatalf("expected DegradedWorkloads condition true, got %v", cond)
		}
	})

	t.Run("infra degraded without running workloads", func(t *testing.T) {
		dev := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "dev-2",
				Labels: map[string]string{gpuDeviceNodeLabelKey: "node-2"},
			},
			Status: v1alpha1.GPUDeviceStatus{
				State: v1alpha1.GPUDeviceStateReady,
			},
		}

		handler := NewWorkloadStatusHandler(testr.New(t))
		handler.SetClient(newWorkloadClient(t, dev))

		inventory := &v1alpha1.GPUNodeState{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: v1alpha1.GPUNodeStateStatus{
				Conditions: []metav1.Condition{{
					Type:   conditionInventoryComplete,
					Status: metav1.ConditionTrue,
				}},
			},
		}

		_, err := handler.HandleNode(context.Background(), inventory)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cond := getCondition(inventory.Status.Conditions, conditionDegradedWorkloads)
		if cond == nil || cond.Status != metav1.ConditionFalse || !strings.Contains(cond.Message, "Infrastructure degraded") {
			t.Fatalf("expected DegradedWorkloads=false with infra degraded message, got %v", cond)
		}
	})
}

func TestWorkloadStatusHelperFunctions(t *testing.T) {
	if got := boolReason(true, "a", "b"); got != "a" {
		t.Fatalf("expected success reason, got %q", got)
	}
	if got := boolReason(false, "a", "b"); got != "b" {
		t.Fatalf("expected failure reason, got %q", got)
	}

	if got := driverMessage(true, componentStatus{}); got != "Driver validation succeeded" {
		t.Fatalf("unexpected driver message: %q", got)
	}
	if got := driverMessage(false, componentStatus{Message: "waiting"}); !strings.Contains(got, "waiting") {
		t.Fatalf("unexpected driver pending message: %q", got)
	}
	if got := driverMessage(false, componentStatus{}); got != "Validator pod has not completed yet" {
		t.Fatalf("unexpected driver default message: %q", got)
	}

	if got := toolkitMessage(true, componentStatus{}); got != "CUDA toolkit validation completed" {
		t.Fatalf("unexpected toolkit message: %q", got)
	}
	if got := toolkitMessage(false, componentStatus{Message: "waiting"}); !strings.Contains(got, "waiting") {
		t.Fatalf("unexpected toolkit pending message: %q", got)
	}
	if got := toolkitMessage(false, componentStatus{}); got != "Toolkit validation is still running" {
		t.Fatalf("unexpected toolkit default message: %q", got)
	}

	if got := monitoringMessage(true, componentStatus{}, componentStatus{}); got != "DCGM exporter is ready" {
		t.Fatalf("unexpected monitoring ok message: %q", got)
	}
	if got := monitoringMessage(false, componentStatus{Ready: false, Message: "dcgm"}, componentStatus{Ready: true}); !strings.Contains(got, "dcgm") {
		t.Fatalf("unexpected monitoring dcgm pending message: %q", got)
	}
	if got := monitoringMessage(false, componentStatus{Ready: true, Message: "ok"}, componentStatus{Ready: false, Message: "exporter"}); !strings.Contains(got, "exporter") {
		t.Fatalf("unexpected monitoring exporter pending message: %q", got)
	}
	if got := monitoringMessage(false, componentStatus{Ready: true}, componentStatus{Ready: true}); got != "DCGM exporter is not ready" {
		t.Fatalf("unexpected monitoring fallback message: %q", got)
	}

	handler := NewWorkloadStatusHandler(testr.New(t))
	if got := handler.componentMessage(true, componentStatus{}, componentStatus{}); got != "Bootstrap workloads are ready" {
		t.Fatalf("unexpected component ok message: %q", got)
	}
	if got := handler.componentMessage(false, componentStatus{Ready: false, Message: "gfd"}, componentStatus{Ready: true}); !strings.Contains(got, "gfd") {
		t.Fatalf("unexpected gfd pending message: %q", got)
	}
	if got := handler.componentMessage(false, componentStatus{Ready: true}, componentStatus{Ready: false, Message: "validator"}); !strings.Contains(got, "validator") {
		t.Fatalf("unexpected validator pending message: %q", got)
	}
	if got := handler.componentMessage(false, componentStatus{Ready: true}, componentStatus{Ready: true}); got != "Bootstrap workloads are still running" {
		t.Fatalf("unexpected component fallback message: %q", got)
	}

	if got := pendingDevicesMessage(1, []string{"dev-a"}); !strings.Contains(got, "GPU device requires validation") || !strings.Contains(got, "dev-a") {
		t.Fatalf("unexpected pendingDevicesMessage: %q", got)
	}
	if got := pendingDevicesMessage(2, nil); !strings.Contains(got, "GPU devices require validation") {
		t.Fatalf("unexpected pendingDevicesMessage plural: %q", got)
	}
}

func TestWorkloadStatusBuildComponentStatusesMessageFallbacks(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t))

	statuses := handler.buildComponentStatuses(validation.Result{Ready: false}, true)
	if statuses[appValidator].Message != "validation workloads not ready" {
		t.Fatalf("unexpected message when present=true: %q", statuses[appValidator].Message)
	}

	statuses = handler.buildComponentStatuses(validation.Result{Ready: false}, false)
	if statuses[appValidator].Message != "validator status unavailable" {
		t.Fatalf("unexpected message when present=false: %q", statuses[appValidator].Message)
	}

	statuses = handler.buildComponentStatuses(validation.Result{Ready: false, Message: "explicit"}, true)
	if statuses[appValidator].Message != "explicit" {
		t.Fatalf("unexpected explicit message: %q", statuses[appValidator].Message)
	}

	statuses = handler.buildComponentStatuses(validation.Result{
		Ready:             true,
		GFDReady:          true,
		DriverReady:       true,
		ToolkitReady:      true,
		DCGMReady:         true,
		DCGMExporterReady: true,
	}, true)
	if statuses[appValidator].Message != "" || !statuses[appValidator].Ready {
		t.Fatalf("expected ready validator with empty message, got %+v", statuses[appValidator])
	}
}

func TestWorkloadStatusEvaluateReadyForPoolingBranches(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t))

	tests := []struct {
		name           string
		devicesPresent bool
		inventory      *v1alpha1.GPUNodeState
		invComplete    bool
		driverReady    bool
		toolkitReady   bool
		componentReady bool
		monitoringOK   bool
		stateCounters  map[v1alpha1.GPUDeviceState]int32
		pendingDevices int
		throttled      []string
		wantReady      bool
		wantReason     string
	}{
		{
			name:           "no devices",
			devicesPresent: false,
			inventory:      &v1alpha1.GPUNodeState{},
			wantReason:     reasonNoDevices,
		},
		{
			name:           "managed disabled",
			devicesPresent: true,
			inventory: &v1alpha1.GPUNodeState{Status: v1alpha1.GPUNodeStateStatus{Conditions: []metav1.Condition{{
				Type:   conditionManagedDisabled,
				Status: metav1.ConditionTrue,
			}}}},
			invComplete: true,
			wantReason:  reasonNodeDisabled,
		},
		{
			name:           "inventory incomplete",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    false,
			wantReason:     reasonInventoryPending,
		},
		{
			name:           "pending devices",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    true,
			toolkitReady:   true,
			componentReady: true,
			monitoringOK:   true,
			pendingDevices: 2,
			throttled:      []string{"dev-a"},
			wantReason:     reasonDevicesPending,
		},
		{
			name:           "driver not ready",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    false,
			wantReason:     reasonDriverNotDetected,
		},
		{
			name:           "toolkit not ready",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    true,
			toolkitReady:   false,
			wantReason:     reasonToolkitNotReady,
		},
		{
			name:           "workloads pending",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    true,
			toolkitReady:   true,
			componentReady: false,
			wantReason:     reasonComponentPending,
		},
		{
			name:           "monitoring unhealthy",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    true,
			toolkitReady:   true,
			componentReady: true,
			monitoringOK:   false,
			wantReason:     reasonMonitoringUnhealthy,
		},
		{
			name:           "faulted devices",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    true,
			toolkitReady:   true,
			componentReady: true,
			monitoringOK:   true,
			stateCounters:  map[v1alpha1.GPUDeviceState]int32{v1alpha1.GPUDeviceStateFaulted: 1},
			wantReason:     reasonDevicesFaulted,
		},
		{
			name:           "all checks passed",
			devicesPresent: true,
			inventory:      &v1alpha1.GPUNodeState{},
			invComplete:    true,
			driverReady:    true,
			toolkitReady:   true,
			componentReady: true,
			monitoringOK:   true,
			wantReady:      true,
			wantReason:     reasonAllChecksPassed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateCounters := tt.stateCounters
			if stateCounters == nil {
				stateCounters = map[v1alpha1.GPUDeviceState]int32{}
			}
			ready, reason, _ := handler.evaluateReadyForPooling(tt.devicesPresent, tt.inventory, tt.invComplete, tt.driverReady, tt.toolkitReady, tt.componentReady, tt.monitoringOK, stateCounters, tt.pendingDevices, tt.throttled)
			if ready != tt.wantReady {
				t.Fatalf("expected ready=%t, got %t", tt.wantReady, ready)
			}
			if reason != tt.wantReason {
				t.Fatalf("expected reason=%s, got %s", tt.wantReason, reason)
			}
		})
	}
}

func TestWorkloadStatusPendingDeviceIDsFallbacks(t *testing.T) {
	devices := []v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-a"},
			Status:     v1alpha1.GPUDeviceStatus{InventoryID: "  ", State: v1alpha1.GPUDeviceStateDiscovered},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-b"},
			Status:     v1alpha1.GPUDeviceStatus{InventoryID: "id-b", State: v1alpha1.GPUDeviceStateValidating},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-c"},
			Status:     v1alpha1.GPUDeviceStatus{InventoryID: "id-c", State: v1alpha1.GPUDeviceStateReady},
		},
	}

	ids := pendingDeviceIDs(devices)
	if len(ids) != 2 {
		t.Fatalf("expected 2 pending IDs, got %#v", ids)
	}
	if ids[0] != "dev-a" || ids[1] != "id-b" {
		t.Fatalf("unexpected sorted IDs: %#v", ids)
	}
}
