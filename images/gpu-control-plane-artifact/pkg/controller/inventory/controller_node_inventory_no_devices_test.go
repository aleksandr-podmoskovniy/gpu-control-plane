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
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

func TestReconcileNodeInventoryMarksNoDevicesDiscovered(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-devices-inventory",
			UID:  types.UID("worker-no-devices-inventory"),
		},
	}

	existingInventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeStateSpec{NodeName: node.Name},
	}

	client := newTestClient(scheme, node, existingInventory)
	reconciler := &Reconciler{
		client:   client,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(32),
		log:      testr.New(t),
	}
	reconciler.setResyncPeriod(defaultResyncPeriod)

	snapshot := nodeSnapshot{
		Managed:         true,
		FeatureDetected: true,
		Labels:          map[string]string{},
	}

	if err := reconciler.inventorySvc().Reconcile(context.Background(), node, snapshot, nil); err != nil {
		t.Fatalf("unexpected reconcileNodeInventory error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeState{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("expected inventory to exist, got error: %v", err)
	}

	condition := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	if condition == nil {
		t.Fatalf("expected inventory condition to be present")
	}
	if condition.Reason != reasonNoDevicesDiscovered || condition.Status != metav1.ConditionFalse {
		t.Fatalf("expected condition=%s/false, got reason=%s status=%s", reasonNoDevicesDiscovered, condition.Reason, condition.Status)
	}
}

func TestReconcileNodeInventorySkipsCreationWhenNoDevicesV2(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-devices-new",
			UID:  types.UID("worker-no-devices-new"),
		},
	}
	client := newTestClient(scheme, node)
	reconciler := &Reconciler{
		client:   client,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(32),
		log:      testr.New(t),
	}

	snapshot := nodeSnapshot{
		Managed:         true,
		FeatureDetected: true,
		Labels:          map[string]string{},
	}

	if err := reconciler.inventorySvc().Reconcile(context.Background(), node, snapshot, nil); err != nil {
		t.Fatalf("unexpected reconcileNodeInventory error: %v", err)
	}

	inventory := &v1alpha1.GPUNodeState{}
	err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, inventory)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be absent, got err=%v obj=%#v", err, inventory)
	}
}

func TestReconcileNodeInventorySkipsCreationWhenNoDevices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-empty"}}
	client := newTestClient(scheme, node)

	reconciler := &Reconciler{
		client: client,
		scheme: scheme,
		log:    testr.New(t),
	}

	snapshot := nodeSnapshot{Devices: nil}
	if err := reconciler.inventorySvc().Reconcile(context.Background(), node, snapshot, nil); err != nil {
		t.Fatalf("reconcileNodeInventory returned error: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "worker-empty"}, &v1alpha1.GPUNodeState{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory not to be created, got %v", err)
	}
}
