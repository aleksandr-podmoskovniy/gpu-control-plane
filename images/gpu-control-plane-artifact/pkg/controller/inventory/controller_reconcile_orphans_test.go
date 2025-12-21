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
	"time"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcileDeletesOrphansAndUpdatesManagedFlag(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-b",
			UID:  types.UID("node-worker-b"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"gpu.deckhouse.io/enabled":          "false",
			},
		},
	}

	primary := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-b-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
			Managed:  true,
		},
	}
	orphan := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "obsolete-device",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "99"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
		},
	}

	client := newTestClient(scheme, node, primary, orphan)

	handler := &trackingHandler{name: "noop"}
	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), []contracts.InventoryHandler{handler})
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: orphan.Name}, &v1alpha1.GPUDevice{}); err != nil {
		t.Fatalf("expected orphan device to remain on active node, got err=%v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: primary.Name}, updated); err != nil {
		t.Fatalf("failed to get primary device: %v", err)
	}
	if updated.Status.Managed {
		t.Fatal("expected managed flag to be false after reconcile")
	}
	if updated.Labels[deviceIndexLabelKey] != "0" {
		t.Fatalf("expected index label to remain 0, got %s", updated.Labels[deviceIndexLabelKey])
	}

	inventory := &v1alpha1.GPUNodeState{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory missing: %v", err)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryComplete); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected InventoryComplete=false, got %+v", cond)
	}
}

func TestReconcileNodeNotFoundTriggersCleanup(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)
	baseClient := newTestClient(scheme)

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.client = &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "nodes"}, "missing")
		},
	}
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(32)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing-node"}}); err != nil {
		t.Fatalf("expected reconcile to succeed on missing node: %v", err)
	}
}

func TestReconcileRemovesOrphansWhenNodeDeleting(t *testing.T) {
	scheme := newTestScheme(t)
	ts := metav1.NewTime(time.Now())
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "worker-delete",
			UID:               types.UID("worker-delete"),
			DeletionTimestamp: &ts,
			Finalizers:        []string{"test-finalizer"},
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2203",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	orphan := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan-device"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: node.Name},
	}

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.client = newTestClient(scheme, node, orphan)
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(16)

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got %+v", res)
	}

	err = rec.client.Get(context.Background(), types.NamespacedName{Name: orphan.Name}, &v1alpha1.GPUDevice{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected orphan device to be removed, got %v", err)
	}
}
