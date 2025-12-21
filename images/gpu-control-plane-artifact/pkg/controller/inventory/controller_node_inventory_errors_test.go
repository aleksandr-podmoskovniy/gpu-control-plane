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
	"errors"
	"strings"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcileNodeInventoryStatusPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-status-error",
			UID:  types.UID("status-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.memory":             "40960 MiB",
			},
		},
	}

	baseClient := newTestClient(scheme, node)
	statusErr := errors.New("status patch failed")
	client := &delegatingClient{Client: baseClient, statusWriter: &errorStatusWriter{StatusWriter: baseClient.Status(), err: statusErr}}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, statusErr) {
		t.Fatalf("expected status patch error, got %v", err)
	}
}

func TestReconcileNodeInventoryReturnsGetError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-inv-get"}}
	base := newTestClient(scheme, node)
	getErr := errors.New("inventory get failure")

	client := &delegatingClient{
		Client: base,
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeState); ok {
				return getErr
			}
			return base.Get(ctx, key, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if err := reconciler.inventorySvc().Reconcile(context.Background(), node, nodeSnapshot{}, nil); !errors.Is(err, getErr) {
		t.Fatalf("expected get error to propagate, got %v", err)
	}
}

func TestReconcileNodeInventoryCreateError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-create-fail",
			UID:  types.UID("node-inv-create-fail"),
		},
	}

	devices := []*v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-inv-create-fail-dev0",
				Labels: map[string]string{deviceIndexLabelKey: "0"},
			},
			Status: v1alpha1.GPUDeviceStatus{InventoryID: "worker-inv-create-fail-0"},
		},
	}

	base := newTestClient(scheme, node, devices[0])
	createErr := errors.New("inventory create failure")
	client := &delegatingClient{
		Client: base,
		create: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeState); ok {
				return createErr
			}
			return base.Create(ctx, obj, opts...)
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
	if !errors.Is(err, createErr) {
		t.Fatalf("expected create error, got %v", err)
	}
}

func TestReconcileNodeInventoryPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-patch",
			UID:  types.UID("node-inv-patch"),
		},
	}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "old-node"},
	}

	base := newTestClient(scheme, node, inventory)
	patchErr := errors.New("inventory patch failure")
	client := &delegatingClient{
		Client: base,
		patch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeState); ok {
				return patchErr
			}
			return base.Patch(ctx, obj, patch, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	err = reconciler.inventorySvc().Reconcile(context.Background(), node, nodeSnapshot{}, nil)
	if !errors.Is(err, patchErr) {
		t.Fatalf("expected patch error, got %v", err)
	}
}

func TestReconcileNodeInventoryRefetchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-inv-refetch",
			UID:  types.UID("node-inv-refetch"),
		},
	}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "old"},
	}

	base := newTestClient(scheme, node, inventory)
	var getCalls int
	client := &delegatingClient{
		Client: base,
		patch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			return base.Patch(ctx, obj, patch, opts...)
		},
		get: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*v1alpha1.GPUNodeState); ok {
				getCalls++
				if getCalls > 1 {
					return errors.New("inventory refetch failure")
				}
			}
			return base.Get(ctx, key, obj, opts...)
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
		Devices: []deviceSnapshot{{
			Index:   "0",
			Vendor:  "10de",
			Device:  "1db5",
			Class:   "0302",
			Product: "GPU",
		}},
	}, []*v1alpha1.GPUDevice{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-inv-refetch-device",
			Labels: map[string]string{deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "worker-inv-refetch-0-10de-1db5",
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "GPU",
				MIG:     v1alpha1.GPUMIGConfig{},
			},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "inventory refetch failure") {
		t.Fatalf("expected refetch failure, got %v", err)
	}
}
