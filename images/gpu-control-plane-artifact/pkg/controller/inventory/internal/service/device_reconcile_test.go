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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func newTestNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name),
		},
	}
}

func newTestSnapshot() invstate.DeviceSnapshot {
	return invstate.DeviceSnapshot{
		Index:      "0",
		Vendor:     "10de",
		Device:     "2203",
		Class:      "0302",
		PCIAddress: "00000000:65:00.0",
		UUID:       "GPU-1",
		MIG:        v1alpha1.GPUMIGConfig{Capable: true, Strategy: v1alpha1.GPUMIGStrategySingle},
	}
}

func TestDeviceServiceReconcileCreateBranches(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-create")
	snapshot := newTestSnapshot()
	approval := invstate.DeviceApprovalPolicy{Mode: moduleconfig.DeviceApprovalModeAutomatic}

	t.Run("success", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		rec := record.NewFakeRecorder(10)

		svc := NewDeviceService(base, scheme, rec, nil)

		device, res, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, func(d *v1alpha1.GPUDevice, _ invstate.DeviceSnapshot) {
			d.Status.Hardware.Product = "from-detection"
			d.Status.Hardware.PCI.Address = "00000000:65:00.0"
		})
		if err != nil {
			t.Fatalf("Reconcile returned error: %v", err)
		}
		if device == nil {
			t.Fatalf("expected device to be created")
		}
		if res != (contracts.Result{}) {
			t.Fatalf("unexpected result: %+v", res)
		}
		if device.Status.Hardware.Product != "from-detection" {
			t.Fatalf("expected detection to be applied, got %q", device.Status.Hardware.Product)
		}
	})

	t.Run("ownerref error", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		badScheme := runtime.NewScheme()

		svc := NewDeviceService(base, badScheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); err == nil {
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

		svc := NewDeviceService(cl, scheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})

	t.Run("handler error", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		handlerErr := errors.New("handler boom")
		svc := NewDeviceService(base, scheme, nil, []contracts.InventoryHandler{
			namedErrorHandler{name: "handler-error", err: handlerErr},
		})

		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); !errors.Is(err, handlerErr) {
			t.Fatalf("expected error %v, got %v", handlerErr, err)
		}
	})

	t.Run("status update conflict requeues", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		cl := &hookClient{
			Client: base,
			status: hookStatusWriter{
				base: base.Status(),
				update: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
					return apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpudevices"}, "conflict", errors.New("conflict"))
				},
			},
		}

		svc := NewDeviceService(cl, scheme, nil, nil)
		device, res, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil)
		if err != nil {
			t.Fatalf("expected conflict to be handled, got %v", err)
		}
		if device == nil {
			t.Fatalf("expected device to be returned on conflict")
		}
		if !res.Requeue {
			t.Fatalf("expected requeue on conflict, got %+v", res)
		}
	})

	t.Run("status update error", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		boom := errors.New("status update boom")
		cl := &hookClient{
			Client: base,
			status: hookStatusWriter{
				base: base.Status(),
				update: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
					return boom
				},
			},
		}

		svc := NewDeviceService(cl, scheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})
}

func TestDeviceServiceReconcileFetchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := newTestNode("node-fetch-error")
	base := newTestClient(t, scheme, node)

	boom := errors.New("get failed")
	cl := &hookClient{
		Client: base,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	svc := NewDeviceService(cl, scheme, nil, nil)
	if _, _, err := svc.Reconcile(context.Background(), node, newTestSnapshot(), nil, true, invstate.DeviceApprovalPolicy{}, nil); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}

func TestDeviceServiceReconcileExistingDeviceBranches(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-existing")
	snapshot := newTestSnapshot()
	approval := invstate.DeviceApprovalPolicy{Mode: moduleconfig.DeviceApprovalModeAutomatic}

	t.Run("metadata error", func(t *testing.T) {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: invstate.BuildDeviceName(node.Name, snapshot)}}
		base := newTestClient(t, scheme, node, device)

		badScheme := runtime.NewScheme()
		svc := NewDeviceService(base, badScheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); err == nil {
			t.Fatalf("expected metadata owner reference error")
		}
	})

	t.Run("refetch error after metadata update", func(t *testing.T) {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: invstate.BuildDeviceName(node.Name, snapshot)}}
		base := newTestClient(t, scheme, node, device)

		boom := errors.New("refetch boom")
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

		svc := NewDeviceService(cl, scheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})

	t.Run("handler error", func(t *testing.T) {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: invstate.BuildDeviceName(node.Name, snapshot)}}
		base := newTestClient(t, scheme, node, device)

		handlerErr := errors.New("handler boom")
		svc := NewDeviceService(base, scheme, nil, []contracts.InventoryHandler{
			namedErrorHandler{name: "handler-error", err: handlerErr},
		})

		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); !errors.Is(err, handlerErr) {
			t.Fatalf("expected error %v, got %v", handlerErr, err)
		}
	})

	t.Run("patch success", func(t *testing.T) {
		snap := snapshot
		snap.Product = "A100"

		device := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: invstate.BuildDeviceName(node.Name, snap)},
			Status:     v1alpha1.GPUDeviceStatus{AutoAttach: false},
		}
		base := newTestClient(t, scheme, node, device)
		svc := NewDeviceService(base, scheme, nil, nil)

		got, res, err := svc.Reconcile(ctx, node, snap, map[string]string{}, true, approval, func(d *v1alpha1.GPUDevice, _ invstate.DeviceSnapshot) {
			d.Status.State = v1alpha1.GPUDeviceStateReady
		})
		if err != nil {
			t.Fatalf("Reconcile returned error: %v", err)
		}
		if got == nil {
			t.Fatalf("expected device to be returned")
		}
		if res != (contracts.Result{}) {
			t.Fatalf("unexpected result: %+v", res)
		}
		if got.Status.State != v1alpha1.GPUDeviceStateReady {
			t.Fatalf("expected handler mutation to be present, got %s", got.Status.State)
		}
	})

	t.Run("patch conflict requeues", func(t *testing.T) {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: invstate.BuildDeviceName(node.Name, snapshot)}}
		base := newTestClient(t, scheme, node, device)

		cl := &hookClient{
			Client: base,
			status: hookStatusWriter{
				base: base.Status(),
				patch: func(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
					return apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpudevices"}, "conflict", errors.New("conflict"))
				},
			},
		}

		svc := NewDeviceService(cl, scheme, nil, nil)
		device, res, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil)
		if err != nil {
			t.Fatalf("expected conflict to be handled, got %v", err)
		}
		if device == nil {
			t.Fatalf("expected device to be returned on conflict")
		}
		if !res.Requeue {
			t.Fatalf("expected requeue on conflict, got %+v", res)
		}
	})

	t.Run("patch error", func(t *testing.T) {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: invstate.BuildDeviceName(node.Name, snapshot)}}
		base := newTestClient(t, scheme, node, device)

		boom := errors.New("patch boom")
		cl := &hookClient{
			Client: base,
			status: hookStatusWriter{
				base: base.Status(),
				patch: func(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
					return boom
				},
			},
		}

		svc := NewDeviceService(cl, scheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); !errors.Is(err, boom) {
			t.Fatalf("expected error %v, got %v", boom, err)
		}
	})

	t.Run("no changes skips patch", func(t *testing.T) {
		deviceName := invstate.BuildDeviceName(node.Name, snapshot)
		device := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name: deviceName,
				Labels: map[string]string{
					invconsts.DeviceNodeLabelKey:  node.Name,
					invconsts.DeviceIndexLabelKey: snapshot.Index,
				},
			},
			Status: v1alpha1.GPUDeviceStatus{
				NodeName:    node.Name,
				InventoryID: invstate.BuildInventoryID(node.Name, snapshot),
				Managed:     true,
				AutoAttach:  true,
				Hardware: v1alpha1.GPUDeviceHardware{
					UUID: snapshot.UUID,
					PCI: v1alpha1.PCIAddress{
						Vendor:  snapshot.Vendor,
						Device:  snapshot.Device,
						Class:   snapshot.Class,
						Address: "0000:65:00.0",
					},
					MIG: snapshot.MIG,
				},
				State: v1alpha1.GPUDeviceStateDiscovered,
			},
		}
		if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
			t.Fatalf("set owner: %v", err)
		}

		base := newTestClient(t, scheme, node, device)
		cl := &hookClient{
			Client: base,
			status: hookStatusWriter{
				base: base.Status(),
				patch: func(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
					return errors.New("unexpected patch")
				},
			},
		}

		svc := NewDeviceService(cl, scheme, nil, nil)
		if _, _, err := svc.Reconcile(ctx, node, snapshot, nil, true, approval, nil); err != nil {
			t.Fatalf("expected no patch, got %v", err)
		}
	})
}
