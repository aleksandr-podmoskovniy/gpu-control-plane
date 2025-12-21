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
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func findCondition(conds []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == conditionType {
			return &conds[i]
		}
	}
	return nil
}

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

	svc := NewInventoryService(base, scheme, record.NewFakeRecorder(10))
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
	cond := findCondition(inventory.Status.Conditions, invconsts.ConditionInventoryComplete)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != invconsts.ReasonInventorySynced {
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
	svc := NewInventoryService(base, scheme, record.NewFakeRecorder(10))

	t.Run("feature missing", func(t *testing.T) {
		snap := invstate.NodeSnapshot{FeatureDetected: false, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}
		if err := svc.Reconcile(ctx, node, snap, []*v1alpha1.GPUDevice{{}}); err != nil {
			t.Fatalf("Reconcile returned error: %v", err)
		}
		got := &v1alpha1.GPUNodeState{}
		if err := base.Get(ctx, types.NamespacedName{Name: node.Name}, got); err != nil {
			t.Fatalf("get inventory: %v", err)
		}
		cond := findCondition(got.Status.Conditions, invconsts.ConditionInventoryComplete)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != invconsts.ReasonNodeFeatureMissing {
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
		cond := findCondition(got.Status.Conditions, invconsts.ConditionInventoryComplete)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != invconsts.ReasonNoDevicesDiscovered {
			t.Fatalf("unexpected condition: %+v", cond)
		}
	})
}

func TestInventoryServiceReconcileSkipsPatchWhenUnchanged(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-noop")

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: node.Name},
		Status: v1alpha1.GPUNodeStateStatus{Conditions: []metav1.Condition{{
			Type:               invconsts.ConditionInventoryComplete,
			Status:             metav1.ConditionTrue,
			Reason:             invconsts.ReasonInventorySynced,
			Message:            "inventory data collected",
			ObservedGeneration: 0,
		}}},
	}
	if err := controllerutil.SetOwnerReference(node, inventory, scheme); err != nil {
		t.Fatalf("set owner: %v", err)
	}

	base := newTestClient(t, scheme, node, inventory)
	cl := &hookClient{
		Client: base,
		status: hookStatusWriter{
			base: base.Status(),
			patch: func(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
				return errors.New("unexpected status patch")
			},
		},
	}

	svc := NewInventoryService(cl, scheme, nil)
	snapshot := invstate.NodeSnapshot{FeatureDetected: true, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}
	if err := svc.Reconcile(ctx, node, snapshot, []*v1alpha1.GPUDevice{{}}); err != nil {
		t.Fatalf("expected no status patch, got %v", err)
	}
}

func TestInventoryServiceReconcileSpecPatchAndErrors(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-spec-patch")

	t.Run("success", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{
			ObjectMeta: metav1.ObjectMeta{Name: node.Name},
			Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "wrong"},
		}
		base := newTestClient(t, scheme, node, inventory)

		svc := NewInventoryService(base, scheme, nil)
		snapshot := invstate.NodeSnapshot{FeatureDetected: true, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}
		if err := svc.Reconcile(ctx, node, snapshot, []*v1alpha1.GPUDevice{{}}); err != nil {
			t.Fatalf("Reconcile returned error: %v", err)
		}
		got := &v1alpha1.GPUNodeState{}
		if err := base.Get(ctx, types.NamespacedName{Name: node.Name}, got); err != nil {
			t.Fatalf("get inventory: %v", err)
		}
		if got.Spec.NodeName != node.Name {
			t.Fatalf("expected nodeName=%q, got %q", node.Name, got.Spec.NodeName)
		}
	})

	t.Run("patch error", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{
			ObjectMeta: metav1.ObjectMeta{Name: node.Name},
			Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "wrong"},
		}
		base := newTestClient(t, scheme, node, inventory)

		boom := errors.New("patch boom")
		cl := &hookClient{
			Client: base,
			patch: func(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
				return boom
			},
		}
		svc := NewInventoryService(cl, scheme, nil)
		if err := svc.Reconcile(ctx, node, invstate.NodeSnapshot{FeatureDetected: true, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}, []*v1alpha1.GPUDevice{{}}); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})

	t.Run("get error after patch", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{
			ObjectMeta: metav1.ObjectMeta{Name: node.Name},
			Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "wrong"},
		}
		base := newTestClient(t, scheme, node, inventory)

		boom := errors.New("get boom")
		getCalls := 0
		cl := &hookClient{
			Client: base,
			get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				getCalls++
				if getCalls == 1 {
					return base.Get(ctx, key, obj, opts...)
				}
				return boom
			},
		}
		svc := NewInventoryService(cl, scheme, nil)
		if err := svc.Reconcile(ctx, node, invstate.NodeSnapshot{FeatureDetected: true, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}, []*v1alpha1.GPUDevice{{}}); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})
}

func TestInventoryServiceReconcileCreateAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-create-error")
	snapshot := invstate.NodeSnapshot{FeatureDetected: true, Devices: []invstate.DeviceSnapshot{{Index: "0"}}}
	devices := []*v1alpha1.GPUDevice{{}}

	t.Run("fetch error", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		boom := errors.New("get boom")
		cl := &hookClient{
			Client: base,
			get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
				return boom
			},
		}
		svc := NewInventoryService(cl, scheme, nil)
		if err := svc.Reconcile(ctx, node, snapshot, devices); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})

	t.Run("ownerref error", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		badScheme := runtime.NewScheme()
		svc := NewInventoryService(base, badScheme, nil)
		if err := svc.Reconcile(ctx, node, snapshot, devices); err == nil {
			t.Fatalf("expected owner reference error")
		}
	})

	t.Run("create error", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		boom := errors.New("create boom")
		cl := &hookClient{
			Client: base,
			create: func(context.Context, client.Object, ...client.CreateOption) error {
				return boom
			},
		}
		svc := NewInventoryService(cl, scheme, nil)
		if err := svc.Reconcile(ctx, node, snapshot, devices); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})

	t.Run("ownerref error on update", func(t *testing.T) {
		inv := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: node.Name}, Spec: v1alpha1.GPUNodeStateSpec{NodeName: node.Name}}
		base := newTestClient(t, scheme, node, inv)
		badScheme := runtime.NewScheme()
		svc := NewInventoryService(base, badScheme, nil)
		if err := svc.Reconcile(ctx, node, snapshot, devices); err == nil {
			t.Fatalf("expected owner reference error")
		}
	})

	t.Run("status patch error", func(t *testing.T) {
		inv := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: node.Name}, Spec: v1alpha1.GPUNodeStateSpec{NodeName: node.Name}}
		if err := controllerutil.SetOwnerReference(node, inv, scheme); err != nil {
			t.Fatalf("set owner: %v", err)
		}
		base := newTestClient(t, scheme, node, inv)

		boom := errors.New("status patch boom")
		cl := &hookClient{
			Client: base,
			status: hookStatusWriter{
				base: base.Status(),
				patch: func(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
					return boom
				},
			},
		}

		svc := NewInventoryService(cl, scheme, nil)
		if err := svc.Reconcile(ctx, node, snapshot, devices); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})
}

func TestInventoryServiceUpdateDeviceMetrics(t *testing.T) {
	svc := &InventoryService{}
	nodeName := "node-metrics"
	svc.UpdateDeviceMetrics(nodeName, []*v1alpha1.GPUDevice{
		{Status: v1alpha1.GPUDeviceStatus{State: ""}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady}},
	})
}
