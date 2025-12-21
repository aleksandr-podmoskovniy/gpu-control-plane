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
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcileNodeInventoryUpdatesSpec(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inventory-update",
			UID:  types.UID("node-inventory-update"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "stale"},
	}

	baseClient := newTestClient(scheme, node, inventory)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = baseClient
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	updated := &v1alpha1.GPUNodeState{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: node.Name}, updated); err != nil {
		t.Fatalf("failed to get updated inventory: %v", err)
	}
	if updated.Spec.NodeName != node.Name {
		t.Fatalf("expected inventory spec node updated, got %s", updated.Spec.NodeName)
	}
	if len(updated.OwnerReferences) == 0 || updated.OwnerReferences[0].UID != node.UID {
		t.Fatalf("expected owner reference to be set")
	}
}

func TestReconcileNodeInventoryPatchesSpecAndOwnerRef(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-patch",
			UID:  types.UID("uid-worker-patch"),
		},
	}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-patch"},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "stale"},
	}
	client := newTestClient(scheme, node, inventory)
	reconciler := &Reconciler{
		client:   client,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(32),
		log:      testr.New(t),
	}

	snapshot := nodeSnapshot{
		Devices: []deviceSnapshot{{
			Index:      "0",
			Vendor:     "10de",
			Device:     "2203",
			Class:      "0300",
			Product:    "GPU",
			PCIAddress: "0000:01:00.0",
		}},
		FeatureDetected: true,
	}
	devices := []*v1alpha1.GPUDevice{{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-patch-0-10de-2203"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "worker-patch",
			InventoryID: "worker-patch-0-10de-2203",
		},
	}}

	if err := reconciler.inventorySvc().Reconcile(context.Background(), node, snapshot, devices); err != nil {
		t.Fatalf("reconcileNodeInventory returned error: %v", err)
	}

	updated := &v1alpha1.GPUNodeState{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "worker-patch"}, updated); err != nil {
		t.Fatalf("expected inventory fetched: %v", err)
	}
	if updated.Spec.NodeName != "worker-patch" {
		t.Fatalf("expected NodeName patched, got %s", updated.Spec.NodeName)
	}
	if len(updated.OwnerReferences) == 0 || updated.OwnerReferences[0].Name != node.Name {
		t.Fatalf("expected owner reference to node, got %+v", updated.OwnerReferences)
	}
	if cond := getCondition(updated.Status.Conditions, conditionInventoryComplete); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InventoryComplete=true after reconcile, got %+v", cond)
	}
}
