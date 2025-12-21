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
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestReconcileCleanupOnMissingNode(t *testing.T) {
	scheme := newTestScheme(t)
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-c-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-c", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "worker-c",
		},
	}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-c"},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "worker-c"},
	}

	client := newTestClient(scheme, device, inventory)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-c"}}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: device.Name}, &v1alpha1.GPUDevice{}); err != nil {
		t.Fatalf("expected device to remain (ownerRef cleanup), err=%v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: inventory.Name}, &v1alpha1.GPUNodeState{}); err != nil {
		t.Fatalf("expected inventory to remain (ownerRef cleanup), err=%v", err)
	}
}

func TestReconcileDeletesExistingInventoryWhenDevicesDisappear(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-stale",
			UID:  types.UID("worker-stale"),
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "stale-device",
		},
	}
	device.Status.NodeName = node.Name
	device.Status.InventoryID = "stale-inventory"

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Spec: v1alpha1.GPUNodeStateSpec{
			NodeName: node.Name,
		},
	}

	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			Labels: map[string]string{
				nodeFeatureNodeNameLabel: node.Name,
			},
		},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{},
		},
	}

	client := newTestClient(scheme, node, feature, device, inventory)

	invmetrics.InventoryDevicesSet(node.Name, 5)
	invmetrics.InventoryConditionSet(node.Name, conditionInventoryComplete, true)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	ctx := context.Background()
	res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no follow-up when node no longer has devices, got %+v", res)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: device.Name}, &v1alpha1.GPUDevice{}); err != nil {
		t.Fatalf("expected GPUDevice to remain for live node, got %v", err)
	}
	persisted := &v1alpha1.GPUNodeState{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, persisted); err != nil {
		t.Fatalf("expected inventory to persist, got error: %v", err)
	}
	if cond := getCondition(persisted.Status.Conditions, conditionInventoryComplete); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected InventoryComplete=false after cleanup, got %+v", cond)
	}
	if value, ok := gaugeValue(t, invmetrics.InventoryDevicesTotalMetric, map[string]string{"node": node.Name}); !ok || value != 0 {
		t.Fatalf("expected devices gauge 0, got %f", value)
	}
}
