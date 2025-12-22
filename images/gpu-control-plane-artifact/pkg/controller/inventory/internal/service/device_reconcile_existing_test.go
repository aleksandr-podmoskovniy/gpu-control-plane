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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

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
		svc := NewDeviceService(base, scheme, nil, []DeviceHandler{
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
		if res != (reconcile.Result{}) {
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
					invstate.DeviceNodeLabelKey:  node.Name,
					invstate.DeviceIndexLabelKey: snapshot.Index,
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
