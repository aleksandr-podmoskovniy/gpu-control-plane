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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestReconcileHandlesNodeFeatureMissing(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-missing-feature",
			UID:  types.UID("node-worker-missing-feature"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	client := newTestClient(scheme, node)
	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
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
	if res.RequeueAfter != defaultResyncPeriod {
		t.Fatalf("expected default resync period (%s), got %+v", defaultResyncPeriod, res)
	}

	inventory := &v1alpha1.GPUNodeState{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("expected inventory to be created, got error: %v", err)
	}
	condition := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	if condition == nil {
		t.Fatalf("expected inventory condition to be set")
	}
	if condition.Status != metav1.ConditionFalse || condition.Reason != reasonNodeFeatureMissing {
		t.Fatalf("expected inventory condition (false, %s), got status=%s reason=%s", reasonNodeFeatureMissing, condition.Status, condition.Reason)
	}
}

func TestReconcileHandlesNoDevicesDiscovered(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-no-devices",
			UID:  types.UID("worker-no-devices"),
		},
	}
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"nvidia.com/gpu.driver":        "535.86.05",
				"nvidia.com/cuda.driver.major": "12",
				"nvidia.com/cuda.driver.minor": "2",
			},
		},
	}

	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID},
			},
		},
		Spec: v1alpha1.GPUNodeStateSpec{NodeName: node.Name},
	}

	client := newTestClient(scheme, node, feature, inventory)
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("expected no resync when node lacks devices, got %+v", res)
	}

	updated := &v1alpha1.GPUNodeState{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, updated); err != nil {
		t.Fatalf("expected inventory to exist, got error: %v", err)
	}
	cond := apimeta.FindStatusCondition(updated.Status.Conditions, conditionInventoryComplete)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonNoDevicesDiscovered {
		t.Fatalf("expected inventory condition (false, %s), got %+v", reasonNoDevicesDiscovered, cond)
	}
	if value, ok := gaugeValue(t, invmetrics.InventoryDevicesTotalMetric, map[string]string{"node": node.Name}); !ok || value != 0 {
		t.Fatalf("expected devices gauge 0, got %f", value)
	}
}
