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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestDeviceServiceReconcileCreateBranches(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := newTestNode("node-create")
	snapshot := newTestSnapshot()
	approval := invstate.DeviceApprovalPolicy{Mode: moduleconfig.DeviceApprovalModeAutomatic}

	t.Run("success", func(t *testing.T) {
		base := newTestClient(t, scheme, node)
		recorder := newTestRecorderLogger(10)
		svc := NewDeviceService(base, scheme, recorder, nil)

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
		if res != (reconcile.Result{}) {
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
		svc := NewDeviceService(base, scheme, nil, []DeviceHandler{
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
