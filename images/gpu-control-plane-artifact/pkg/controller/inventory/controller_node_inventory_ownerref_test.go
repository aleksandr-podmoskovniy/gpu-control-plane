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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

func TestReconcileNodeInventoryOwnerReferenceError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-owner-ref",
			UID:  types.UID("owner-ref"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	client := newTestClient(scheme, node)
	reconciler := &Reconciler{
		client:          client,
		scheme:          runtime.NewScheme(),
		recorder:        record.NewFakeRecorder(32),
		fallbackManaged: ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/enabled", EnabledByDefault: true},
		log:             testr.New(t),
	}

	snapshot := buildNodeSnapshot(node, nil, reconciler.fallbackManaged)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "dev-owner-ref",
			Labels: map[string]string{deviceIndexLabelKey: "00"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "owner-ref-00",
		},
	}

	err := reconciler.inventorySvc().Reconcile(context.Background(), node, snapshot, []*v1alpha1.GPUDevice{device})
	if err == nil {
		t.Fatalf("expected owner reference error due to missing scheme registration")
	}
}

func TestReconcileNodeInventoryOwnerReferenceCreateError(t *testing.T) {
	scheme := runtime.NewScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-create"}}
	client := newTestClient(newTestScheme(t), node)

	devices := []*v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-create-dev0",
				Labels: map[string]string{deviceIndexLabelKey: "0"},
			},
			Status: v1alpha1.GPUDeviceStatus{InventoryID: "worker-inv-create-0"},
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.inventorySvc().Reconcile(context.Background(), node, nodeSnapshot{
		Devices: []deviceSnapshot{{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}},
	}, devices)
	if err == nil {
		t.Fatal("expected owner reference error on create")
	}
}

func TestReconcileNodeInventoryOwnerReferenceUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-update"}}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: node.Name},
	}
	client := newTestClient(newTestScheme(t), node, inventory)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.inventorySvc().Reconcile(context.Background(), node, nodeSnapshot{}, nil)
	if err == nil {
		t.Fatal("expected owner reference error on update")
	}
}
