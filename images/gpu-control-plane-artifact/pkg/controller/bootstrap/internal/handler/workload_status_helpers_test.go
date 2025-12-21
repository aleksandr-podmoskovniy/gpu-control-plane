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

package handler

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestWorkloadStatusHandlerName(t *testing.T) {
	h := &WorkloadStatusHandler{}
	if h.Name() != "workload-status" {
		t.Fatalf("unexpected name: %s", h.Name())
	}
}

func TestWorkloadsDegradedMessageBranches(t *testing.T) {
	if msg := workloadsDegradedMessage(false, false); msg != "no GPU workloads detected on node" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if msg := workloadsDegradedMessage(true, true); msg != "GPU workloads are running while infrastructure is not ready" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if msg := workloadsDegradedMessage(true, false); msg != "GPU workloads are running on node" {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestConditionMessageBranches(t *testing.T) {
	if msg := conditionMessage(true, "ok", "details"); msg != "ok" {
		t.Fatalf("expected ok message, got %s", msg)
	}
	if msg := conditionMessage(false, "ok", "details"); msg != "details" {
		t.Fatalf("expected details message, got %s", msg)
	}
	if msg := conditionMessage(false, "ok", ""); msg != "not ready" {
		t.Fatalf("expected default not ready message, got %s", msg)
	}
}

func TestPendingDevicesMessageBranches(t *testing.T) {
	if msg := pendingDevicesMessage(1, nil); msg != "1 GPU device requires validation" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if msg := pendingDevicesMessage(2, nil); msg != "2 GPU devices require validation" {
		t.Fatalf("unexpected message: %s", msg)
	}
	if msg := pendingDevicesMessage(2, []string{"b", "a"}); msg != "2 GPU devices require validation (b, a)" {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestPendingDeviceIDsBranches(t *testing.T) {
	devices := []v1alpha1.GPUDevice{
		{ObjectMeta: metav1.ObjectMeta{Name: "ready"}, Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady, InventoryID: "inv-ready"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "needs-validation"}, Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateDiscovered, InventoryID: "  inv-1  "}},
		{ObjectMeta: metav1.ObjectMeta{Name: "fallback-name"}, Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateDiscovered}},
	}

	ids := pendingDeviceIDs(devices)
	if len(ids) != 2 || ids[0] != "fallback-name" || ids[1] != "inv-1" {
		t.Fatalf("unexpected pending IDs: %v", ids)
	}
}

func TestEvaluateReadyForPoolingBranches(t *testing.T) {
	t.Run("no-devices", func(t *testing.T) {
		ready, reason, msg := evaluateReadyForPooling(false, true, true, true, true, nil, 0, nil, "")
		if ready || reason != reasonNoDevices || msg == "" {
			t.Fatalf("unexpected result: ready=%t reason=%s msg=%s", ready, reason, msg)
		}
	})

	t.Run("inventory-incomplete", func(t *testing.T) {
		ready, reason, _ := evaluateReadyForPooling(true, false, true, true, true, nil, 0, nil, "")
		if ready || reason != reasonInventoryIncomplete {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})

	t.Run("faulted-devices", func(t *testing.T) {
		counters := map[v1alpha1.GPUDeviceState]int32{v1alpha1.GPUDeviceStateFaulted: 2}
		ready, reason, _ := evaluateReadyForPooling(true, true, true, true, true, counters, 0, nil, "")
		if ready || reason != reasonDevicesFaulted {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})

	t.Run("pending-devices", func(t *testing.T) {
		ready, reason, msg := evaluateReadyForPooling(true, true, true, true, true, nil, 2, []string{"a"}, "")
		if ready || reason != reasonPendingDevices || msg == "" {
			t.Fatalf("unexpected result: ready=%t reason=%s msg=%s", ready, reason, msg)
		}
	})

	t.Run("driver-not-ready", func(t *testing.T) {
		ready, reason, msg := evaluateReadyForPooling(true, true, false, true, true, nil, 0, nil, "driver down")
		if ready || reason != reasonDriverNotReady || msg != "driver down" {
			t.Fatalf("unexpected result: ready=%t reason=%s msg=%s", ready, reason, msg)
		}
	})

	t.Run("toolkit-not-ready", func(t *testing.T) {
		ready, reason, msg := evaluateReadyForPooling(true, true, true, false, true, nil, 0, nil, "toolkit down")
		if ready || reason != reasonToolkitNotReady || msg != "toolkit down" {
			t.Fatalf("unexpected result: ready=%t reason=%s msg=%s", ready, reason, msg)
		}
	})

	t.Run("monitoring-not-ready", func(t *testing.T) {
		ready, reason, msg := evaluateReadyForPooling(true, true, true, true, false, nil, 0, nil, "monitoring down")
		if ready || reason != reasonMonitoringNotReady || msg != "monitoring down" {
			t.Fatalf("unexpected result: ready=%t reason=%s msg=%s", ready, reason, msg)
		}
	})

	t.Run("ready", func(t *testing.T) {
		ready, reason, msg := evaluateReadyForPooling(true, true, true, true, true, nil, 0, nil, "")
		if !ready || reason != reasonReady || msg == "" {
			t.Fatalf("unexpected result: ready=%t reason=%s msg=%s", ready, reason, msg)
		}
	})
}
