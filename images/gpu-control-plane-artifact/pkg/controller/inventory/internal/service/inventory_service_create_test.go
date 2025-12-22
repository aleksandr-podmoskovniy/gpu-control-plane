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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

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
				update: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
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
