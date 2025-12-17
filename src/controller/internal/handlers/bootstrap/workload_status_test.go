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
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/indexer"
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

	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDeviceNodeField, func(obj client.Object) []string {
			dev, ok := obj.(*v1alpha1.GPUDevice)
			if !ok || dev.Status.NodeName == "" {
				return nil
			}
			return []string{dev.Status.NodeName}
		})

	if len(objs) > 0 {
		builder = builder.WithRuntimeObjects(objs...)
	}

	return builder.Build()
}

func getCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func TestWorkloadStatusHandlerSetsReadyForPoolingTrueWhenAllChecksPassed(t *testing.T) {
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-1",
			InventoryID: "inv-1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}

	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t, dev))

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1alpha1.GPUNodeStateStatus{
			Conditions: []metav1.Condition{{Type: conditionInventoryComplete, Status: metav1.ConditionTrue}},
		},
	}

	ctx := validation.ContextWithStatus(context.Background(), validation.Result{
		DriverReady:       true,
		ToolkitReady:      true,
		GFDReady:          true,
		DCGMReady:         true,
		DCGMExporterReady: true,
	})

	res, err := handler.HandleNode(ctx, inventory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", res)
	}

	for _, c := range []string{conditionDriverReady, conditionToolkitReady, conditionMonitoringReady} {
		cond := getCondition(inventory.Status.Conditions, c)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Fatalf("expected %s condition true, got %v", c, cond)
		}
	}

	cond := getCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != reasonReady {
		t.Fatalf("expected ReadyForPooling=true with reason %s, got %v", reasonReady, cond)
	}
}

func TestWorkloadStatusHandlerReadyForPoolingFalseWhenDevicePendingValidation(t *testing.T) {
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-2"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-2",
			InventoryID: "inv-2",
			State:       v1alpha1.GPUDeviceStateDiscovered,
		},
	}

	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t, dev))

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status: v1alpha1.GPUNodeStateStatus{
			Conditions: []metav1.Condition{{Type: conditionInventoryComplete, Status: metav1.ConditionTrue}},
		},
	}

	ctx := validation.ContextWithStatus(context.Background(), validation.Result{
		DriverReady:       true,
		ToolkitReady:      true,
		GFDReady:          true,
		DCGMReady:         true,
		DCGMExporterReady: true,
	})

	_, err := handler.HandleNode(ctx, inventory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := getCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonPendingDevices || !strings.Contains(cond.Message, "inv-2") {
		t.Fatalf("expected ReadyForPooling=false with reason %s and inventoryID in message, got %v", reasonPendingDevices, cond)
	}
}

func TestWorkloadStatusHandlerReadyForPoolingFalseWhenInventoryIncomplete(t *testing.T) {
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-3"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-3",
			State:    v1alpha1.GPUDeviceStateReady,
		},
	}

	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t, dev))

	inventory := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node-3"}}

	ctx := validation.ContextWithStatus(context.Background(), validation.Result{
		DriverReady:       true,
		ToolkitReady:      true,
		GFDReady:          true,
		DCGMReady:         true,
		DCGMExporterReady: true,
	})

	_, err := handler.HandleNode(ctx, inventory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := getCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonInventoryIncomplete {
		t.Fatalf("expected ReadyForPooling=false with reason %s, got %v", reasonInventoryIncomplete, cond)
	}
}

func TestWorkloadStatusHandlerSetsWorkloadsDegradedWhenWorkloadsRunningButInfraNotReady(t *testing.T) {
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-4"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-4",
			State:    v1alpha1.GPUDeviceStateAssigned,
		},
	}

	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t, dev))

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-4"},
		Status: v1alpha1.GPUNodeStateStatus{
			Conditions: []metav1.Condition{{Type: conditionInventoryComplete, Status: metav1.ConditionTrue}},
		},
	}

	_, err := handler.HandleNode(context.Background(), inventory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	degraded := getCondition(inventory.Status.Conditions, conditionWorkloadsDegraded)
	if degraded == nil || degraded.Status != metav1.ConditionTrue || degraded.Reason != reasonWorkloadsDegraded {
		t.Fatalf("expected WorkloadsDegraded=true with reason %s, got %v", reasonWorkloadsDegraded, degraded)
	}

	ready := getCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != reasonDriverNotReady {
		t.Fatalf("expected ReadyForPooling=false with reason %s, got %v", reasonDriverNotReady, ready)
	}
}

func TestWorkloadStatusHandlerSkipsWhenNodeNameEmpty(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t))
	handler.SetClient(newWorkloadClient(t))

	res, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestWorkloadStatusHandlerClientAndListErrors(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t))

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
