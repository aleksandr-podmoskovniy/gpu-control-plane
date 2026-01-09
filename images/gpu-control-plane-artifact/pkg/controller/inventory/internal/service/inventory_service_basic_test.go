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

package service

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func TestInventoryServiceReconcileNoDevicesNoInventory(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-empty")
	base := newTestClient(t, scheme, node)

	svc := NewInventoryService(base, scheme, nil)
	if err := svc.Reconcile(ctx, node, invstate.NodeSnapshot{}, nil); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	err := base.Get(ctx, types.NamespacedName{Name: node.Name}, &v1alpha1.GPUNodeState{})
	if err == nil {
		t.Fatalf("expected inventory to not be created")
	}
}

func TestInventoryServiceReconcileCreatesInventoryAndSetsCondition(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-create-inv")
	base := newTestClient(t, scheme, node)

	svc := NewInventoryService(base, scheme, newTestRecorderLogger(10))
	snapshot := invstate.NodeSnapshot{
		FeatureDetected: true,
		Devices:         []invstate.DeviceSnapshot{{Index: "0"}},
	}
	devices := []*v1alpha1.GPUDevice{{}}

	if err := svc.Reconcile(ctx, node, snapshot, devices); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeState{}
	if err := base.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if inventory.Spec.NodeName != node.Name {
		t.Fatalf("expected nodeName=%q, got %q", node.Name, inventory.Spec.NodeName)
	}
	cond := findCondition(inventory.Status.Conditions, invstate.ConditionInventoryComplete)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != invstate.ReasonInventorySynced {
		t.Fatalf("unexpected condition: %+v", cond)
	}
}

func TestInventoryServiceReconcileFeatureMissingAndNoDevicesBranches(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-branches")

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: node.Name},
	}
	if err := controllerutil.SetOwnerReference(node, inventory, scheme); err != nil {
		t.Fatalf("set owner: %v", err)
	}

	base := newTestClient(t, scheme, node, inventory)
	svc := NewInventoryService(base, scheme, newTestRecorderLogger(10))

	t.Run("feature missing", func(t *testing.T) {
		snap := invstate.NodeSnapshot{FeatureDetected: false, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}
		if err := svc.Reconcile(ctx, node, snap, []*v1alpha1.GPUDevice{{}}); err != nil {
			t.Fatalf("Reconcile returned error: %v", err)
		}
		got := &v1alpha1.GPUNodeState{}
		if err := base.Get(ctx, types.NamespacedName{Name: node.Name}, got); err != nil {
			t.Fatalf("get inventory: %v", err)
		}
		cond := findCondition(got.Status.Conditions, invstate.ConditionInventoryComplete)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != invstate.ReasonNodeFeatureMissing {
			t.Fatalf("unexpected condition: %+v", cond)
		}
	})

	t.Run("no devices discovered", func(t *testing.T) {
		snap := invstate.NodeSnapshot{FeatureDetected: true, Devices: nil}
		if err := svc.Reconcile(ctx, node, snap, []*v1alpha1.GPUDevice{{}}); err != nil {
			t.Fatalf("Reconcile returned error: %v", err)
		}
		got := &v1alpha1.GPUNodeState{}
		if err := base.Get(ctx, types.NamespacedName{Name: node.Name}, got); err != nil {
			t.Fatalf("get inventory: %v", err)
		}
		cond := findCondition(got.Status.Conditions, invstate.ConditionInventoryComplete)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != invstate.ReasonNoDevicesDiscovered {
			t.Fatalf("unexpected condition: %+v", cond)
		}
	})
}
