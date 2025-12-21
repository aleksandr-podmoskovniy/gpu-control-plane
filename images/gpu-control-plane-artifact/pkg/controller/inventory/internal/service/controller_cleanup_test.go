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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type cleanupDelegatingClient struct {
	client.Client
	get    func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error
	list   func(context.Context, client.ObjectList, ...client.ListOption) error
	delete func(context.Context, client.Object, ...client.DeleteOption) error
}

func (d *cleanupDelegatingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if d.get != nil {
		return d.get(ctx, key, obj, opts...)
	}
	return d.Client.Get(ctx, key, obj, opts...)
}

func (d *cleanupDelegatingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if d.list != nil {
		return d.list(ctx, list, opts...)
	}
	return d.Client.List(ctx, list, opts...)
}

func (d *cleanupDelegatingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if d.delete != nil {
		return d.delete(ctx, obj, opts...)
	}
	return d.Client.Delete(ctx, obj, opts...)
}

func TestCleanupNodeDeletesMetrics(t *testing.T) {
	const nodeName = "cleanup-metrics"
	invmetrics.InventoryDevicesSet(nodeName, 2)
	invmetrics.InventoryConditionSet(nodeName, invconsts.ConditionInventoryComplete, true)

	scheme := newTestScheme(t)
	cl := newTestClient(t, scheme)
	svc := NewCleanupService(cl, record.NewFakeRecorder(1))

	if err := svc.CleanupNode(context.Background(), nodeName); err != nil {
		t.Fatalf("cleanupNode returned error: %v", err)
	}
}

func TestDeleteInventoryRemovesResource(t *testing.T) {
	scheme := newTestScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-cleanup"},
	}
	fixtureClient := newTestClient(t, scheme, inventory)
	svc := NewCleanupService(fixtureClient, record.NewFakeRecorder(1))

	if err := svc.DeleteInventory(context.Background(), "node-cleanup"); err != nil {
		t.Fatalf("deleteInventory returned error: %v", err)
	}

	err := fixtureClient.Get(context.Background(), types.NamespacedName{Name: "node-cleanup"}, &v1alpha1.GPUNodeState{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be deleted, got err=%v", err)
	}

	if err := svc.DeleteInventory(context.Background(), "missing-node"); err != nil {
		t.Fatalf("deleteInventory should ignore missing objects, got %v", err)
	}

	baseClient := newTestClient(t, scheme, &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-delete-race"},
	})
	delClient := &cleanupDelegatingClient{
		Client: baseClient,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpunodestates"}, obj.GetName())
		},
	}
	delSvc := NewCleanupService(delClient, record.NewFakeRecorder(1))

	if err := delSvc.DeleteInventory(context.Background(), "node-delete-race"); err != nil {
		t.Fatalf("deleteInventory should ignore not found error from delete, got %v", err)
	}
}

func TestDeleteInventoryPropagatesGetError(t *testing.T) {
	scheme := newTestScheme(t)
	base := newTestClient(t, scheme)
	boom := errors.New("get failure")

	cl := &cleanupDelegatingClient{
		Client: base,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}

	svc := NewCleanupService(cl, record.NewFakeRecorder(1))

	if err := svc.DeleteInventory(context.Background(), "node-error"); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}

func TestCleanupNodeReturnsListError(t *testing.T) {
	scheme := newTestScheme(t)
	listErr := errors.New("list failure")

	cl := &cleanupDelegatingClient{
		Client: newTestClient(t, scheme),
		list: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return listErr
		},
	}

	svc := NewCleanupService(cl, record.NewFakeRecorder(1))
	if err := svc.CleanupNode(context.Background(), "worker-list-error"); !errors.Is(err, listErr) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestCleanupNodeReturnsDeviceDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-0",
			Labels: map[string]string{invconsts.DeviceNodeLabelKey: "worker-delete", invconsts.DeviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "worker-delete"},
	}
	base := newTestClient(t, scheme, device)
	deleteErr := errors.New("device delete failure")

	cl := &cleanupDelegatingClient{
		Client: base,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			switch obj.(type) {
			case *v1alpha1.GPUDevice:
				return deleteErr
			default:
				return base.Delete(ctx, obj, opts...)
			}
		},
	}

	svc := NewCleanupService(cl, record.NewFakeRecorder(1))
	if err := svc.CleanupNode(context.Background(), "worker-delete"); !errors.Is(err, deleteErr) {
		t.Fatalf("expected device delete error, got %v", err)
	}
}

func TestCleanupNodeReturnsInventoryDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-inventory"},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "worker-inventory"},
	}
	base := newTestClient(t, scheme, inventory)
	deleteErr := errors.New("inventory delete failure")

	cl := &cleanupDelegatingClient{
		Client: base,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			switch obj.(type) {
			case *v1alpha1.GPUNodeState:
				return deleteErr
			default:
				return base.Delete(ctx, obj, opts...)
			}
		},
	}

	svc := NewCleanupService(cl, record.NewFakeRecorder(1))
	if err := svc.CleanupNode(context.Background(), "worker-inventory"); !errors.Is(err, deleteErr) {
		t.Fatalf("expected inventory delete error, got %v", err)
	}
}
