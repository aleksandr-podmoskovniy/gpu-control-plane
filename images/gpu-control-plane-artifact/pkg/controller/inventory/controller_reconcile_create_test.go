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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestReconcileCreatesDeviceAndInventory(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-a",
			UID:  types.UID("node-worker-a"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.product":            "NVIDIA A100-PCIE-40GB",
				"nvidia.com/gpu.memory":             "40536 MiB",
				"nvidia.com/gpu.compute.major":      "8",
				"nvidia.com/gpu.compute.minor":      "0",
				"nvidia.com/mig.capable":            "true",
				"nvidia.com/mig.strategy":           "single",
				"nvidia.com/mig-1g.10gb.count":      "2",
			},
		},
	}

	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"nvidia.com/gpu.driver":          "535.86.05",
				"nvidia.com/cuda.driver.major":   "12",
				"nvidia.com/cuda.driver.minor":   "2",
				"gpu.deckhouse.io/toolkit.ready": "true",
			},
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":          "0",
								"uuid":           "GPU-TEST-UUID-0001",
								"precision":      "fp32,fp16",
								"memory.total":   "40536 MiB",
								"compute.major":  "8",
								"compute.minor":  "0",
								"product":        "NVIDIA A100-PCIE-40GB",
								"precision.bf16": "false",
							}},
						},
					},
				},
			},
		},
	}

	handler := &trackingHandler{
		name:   "state-default",
		state:  v1alpha1.GPUDeviceStateReserved,
		result: contracts.Result{RequeueAfter: 10 * time.Second},
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

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), []contracts.InventoryHandler{handler})
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
	if res.Requeue || res.RequeueAfter != 10*time.Second {
		t.Fatalf("unexpected reconcile result: %+v", res)
	}
	if len(handler.handled) != 1 {
		t.Fatalf("expected handler to be invoked once, got %d", len(handler.handled))
	}

	nodeSnapshot := buildNodeSnapshot(node, feature, managedPolicyFrom(module))
	if len(nodeSnapshot.Devices) != 1 {
		t.Fatalf("expected single snapshot, got %d", len(nodeSnapshot.Devices))
	}
	snapshot := nodeSnapshot.Devices[0]

	deviceName := buildDeviceName(node.Name, snapshot)
	device := &v1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if device.Status.NodeName != node.Name {
		t.Fatalf("expected node name %q, got %q", node.Name, device.Status.NodeName)
	}
	if device.Status.InventoryID == "" {
		t.Fatal("inventoryID must be populated")
	}
	if device.Status.Managed != true {
		t.Fatalf("expected managed true, got %v", device.Status.Managed)
	}
	if device.Status.AutoAttach {
		t.Fatalf("expected autoAttach=false for manual approval, got true")
	}
	if device.Status.State != v1alpha1.GPUDeviceStateReserved {
		t.Fatalf("expected state Reserved, got %s", device.Status.State)
	}
	if device.Status.Hardware.PCI.Vendor != "10de" {
		t.Fatalf("unexpected vendor: %s", device.Status.Hardware.PCI.Vendor)
	}
	if device.Status.Hardware.Product != "NVIDIA A100-PCIE-40GB" {
		t.Fatalf("unexpected product: %s", device.Status.Hardware.Product)
	}
	if device.Status.Hardware.UUID != "GPU-TEST-UUID-0001" {
		t.Fatalf("unexpected uuid: %s", device.Status.Hardware.UUID)
	}
	mig := device.Status.Hardware.MIG
	if !mig.Capable {
		t.Fatal("expected MIG capable true")
	}
	if mig.Strategy != v1alpha1.GPUMIGStrategySingle {
		t.Fatalf("unexpected MIG strategy: %s", mig.Strategy)
	}
	if len(mig.ProfilesSupported) != 1 || mig.ProfilesSupported[0] != "1g.10gb" {
		t.Fatalf("unexpected MIG profiles: %+v", mig.ProfilesSupported)
	}
	if len(mig.Types) != 1 || mig.Types[0].Name != "1g.10gb" || mig.Types[0].Count != 2 {
		t.Fatalf("unexpected MIG types: %+v", mig.Types)
	}

	fetchedInventory := &v1alpha1.GPUNodeState{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, fetchedInventory); err != nil {
		t.Fatalf("inventory not found: %v", err)
	}
	if fetchedInventory.Spec.NodeName != node.Name {
		t.Fatalf("inventory spec node mismatch: %q", fetchedInventory.Spec.NodeName)
	}
	if cond := getCondition(fetchedInventory.Status.Conditions, conditionInventoryComplete); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InventoryComplete=true, got %+v", cond)
	}
}

func TestReconcileHandlesNamespacedNodeFeature(t *testing.T) {
	module := defaultModuleSettings()
	scheme := newTestScheme(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-namespaced",
			UID:  types.UID("node-worker-namespaced"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
				"nvidia.com/gpu.product":            "NVIDIA A100-PCIE-40GB",
				"nvidia.com/gpu.memory":             "40536 MiB",
				"nvidia.com/gpu.compute.major":      "8",
				"nvidia.com/gpu.compute.minor":      "0",
				"nvidia.com/mig.capable":            "true",
				"nvidia.com/mig.strategy":           "single",
			},
		},
	}

	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "gpu-operator",
			Name:      node.Name,
			Labels: map[string]string{
				nodeFeatureNodeNameLabel: node.Name,
			},
		},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"nvidia.com/gpu.driver":          "535.86.05",
				"nvidia.com/cuda.driver.major":   "12",
				"nvidia.com/cuda.driver.minor":   "2",
				"gpu.deckhouse.io/toolkit.ready": "true",
			},
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{
					"nvidia.com/gpu": {
						Elements: []nfdv1alpha1.InstanceFeature{
							{Attributes: map[string]string{
								"index":         "0",
								"uuid":          "GPU-TEST-UUID-NAMESPACED",
								"precision":     "fp32,fp16",
								"memory.total":  "40536 MiB",
								"compute.major": "8",
								"compute.minor": "0",
								"product":       "NVIDIA A100-PCIE-40GB",
							}},
						},
					},
				},
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
	handler := &trackingHandler{state: v1alpha1.GPUDeviceStateReserved}
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

	nodeSnapshot := buildNodeSnapshot(node, feature, managedPolicyFrom(module))
	if len(nodeSnapshot.Devices) != 1 {
		t.Fatalf("expected single device snapshot, got %d", len(nodeSnapshot.Devices))
	}

	deviceName := buildDeviceName(node.Name, nodeSnapshot.Devices[0])
	device := &v1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if device.Status.NodeName != node.Name {
		t.Fatalf("expected NodeName %q, got %q", node.Name, device.Status.NodeName)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory not found: %v", err)
	}
	cond := getCondition(inventory.Status.Conditions, conditionInventoryComplete)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != reasonInventorySynced {
		t.Fatalf("expected InventoryComplete=true reason=%s, got %+v", reasonInventorySynced, cond)
	}
}
