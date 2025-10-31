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

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/pkg/apis/nfd/v1alpha1"
)

type trackingHandler struct {
	name    string
	state   gpuv1alpha1.GPUDeviceState
	result  contracts.Result
	handled []string
}

func (h *trackingHandler) Name() string {
	if h.name != "" {
		return h.name
	}
	return "tracking"
}

func (h *trackingHandler) HandleDevice(_ context.Context, device *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
	h.handled = append(h.handled, device.Name)
	if h.state != "" {
		device.Status.State = h.state
	}
	return h.result, nil
}

func defaultModuleSettings() config.ModuleSettings {
	return config.DefaultSystem().Module
}

func managedPolicyFrom(module config.ModuleSettings) ManagedNodesPolicy {
	return ManagedNodesPolicy{
		LabelKey:         module.ManagedNodes.LabelKey,
		EnabledByDefault: module.ManagedNodes.EnabledByDefault,
	}
}

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
		state:  gpuv1alpha1.GPUDeviceStateReserved,
		result: contracts.Result{RequeueAfter: 10 * time.Second},
	}

	client := newTestClient(scheme, node, feature)

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, []contracts.InventoryHandler{handler})
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
	device := &gpuv1alpha1.GPUDevice{}
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
	if device.Status.State != gpuv1alpha1.GPUDeviceStateReserved {
		t.Fatalf("expected state Reserved, got %s", device.Status.State)
	}
	if device.Status.Hardware.PCI.Vendor != "10de" {
		t.Fatalf("unexpected vendor: %s", device.Status.Hardware.PCI.Vendor)
	}
	if device.Status.Hardware.Product != "NVIDIA A100-PCIE-40GB" {
		t.Fatalf("unexpected product: %s", device.Status.Hardware.Product)
	}
	if device.Status.Hardware.MemoryMiB != 40536 {
		t.Fatalf("unexpected memory: %d", device.Status.Hardware.MemoryMiB)
	}
	if device.Status.Hardware.ComputeCapability == nil {
		t.Fatal("compute capability must be set")
	}
	if device.Status.Hardware.ComputeCapability.Major != 8 || device.Status.Hardware.ComputeCapability.Minor != 0 {
		t.Fatalf("unexpected compute capability: %+v", device.Status.Hardware.ComputeCapability)
	}
	mig := device.Status.Hardware.MIG
	if !mig.Capable {
		t.Fatal("expected MIG capable true")
	}
	if mig.Strategy != gpuv1alpha1.GPUMIGStrategySingle {
		t.Fatalf("unexpected MIG strategy: %s", mig.Strategy)
	}
	if len(mig.ProfilesSupported) != 1 || mig.ProfilesSupported[0] != "mig-1g.10gb" {
		t.Fatalf("unexpected MIG profiles: %+v", mig.ProfilesSupported)
	}
	if len(mig.Types) != 1 || mig.Types[0].Name != "mig-1g.10gb" || mig.Types[0].Count != 2 {
		t.Fatalf("unexpected MIG types: %+v", mig.Types)
	}
	if !stringSlicesEqual(device.Status.Hardware.Precision.Supported, []string{"fp16", "fp32"}) {
		t.Fatalf("unexpected precision list: %+v", device.Status.Hardware.Precision.Supported)
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory not found: %v", err)
	}
	if inventory.Spec.NodeName != node.Name {
		t.Fatalf("inventory spec node mismatch: %q", inventory.Spec.NodeName)
	}
	if !inventory.Status.Hardware.Present {
		t.Fatal("inventory should mark hardware present")
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("expected 1 device in inventory, got %d", len(inventory.Status.Hardware.Devices))
	}
	if inventory.Status.Hardware.Devices[0].InventoryID != device.Status.InventoryID {
		t.Fatalf("inventory device id mismatch")
	}
	inventoryDevice := inventory.Status.Hardware.Devices[0]
	if inventoryDevice.Product != "NVIDIA A100-PCIE-40GB" {
		t.Fatalf("unexpected inventory product: %s", inventoryDevice.Product)
	}
	if inventoryDevice.MIG.Strategy != gpuv1alpha1.GPUMIGStrategySingle {
		t.Fatalf("unexpected inventory MIG strategy: %s", inventoryDevice.MIG.Strategy)
	}
	if len(inventoryDevice.MIG.Types) != 1 || inventoryDevice.MIG.Types[0].Name != "mig-1g.10gb" || inventoryDevice.MIG.Types[0].Count != 2 {
		t.Fatalf("unexpected inventory MIG types: %+v", inventoryDevice.MIG.Types)
	}
	if inventoryDevice.UUID != "GPU-TEST-UUID-0001" {
		t.Fatalf("unexpected inventory UUID: %s", inventoryDevice.UUID)
	}
	if !stringSlicesEqual(inventoryDevice.Precision.Supported, []string{"fp16", "fp32"}) {
		t.Fatalf("unexpected inventory precision: %+v", inventoryDevice.Precision.Supported)
	}
	if inventory.Status.Driver.Version != "535.86.05" {
		t.Fatalf("unexpected driver version: %s", inventory.Status.Driver.Version)
	}
	if inventory.Status.Driver.CUDAVersion != "12.2" {
		t.Fatalf("unexpected cuda version: %s", inventory.Status.Driver.CUDAVersion)
	}
	if !inventory.Status.Driver.ToolkitReady {
		t.Fatal("expected driver toolkit ready true")
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryIncomplete); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected InventoryIncomplete=false, got %+v", cond)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionManagedDisabled); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected ManagedDisabled=false, got %+v", cond)
	}
}

func TestReconcileSchedulesDefaultResync(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-resync"}}

	client := newTestClient(scheme, node)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if res.Requeue {
		t.Fatalf("expected no immediate requeue, got %+v", res)
	}
	if res.RequeueAfter != defaultResyncPeriod {
		t.Fatalf("expected default resync period %s, got %s", defaultResyncPeriod, res.RequeueAfter)
	}
}

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

	primary := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-b-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
			Managed:  true,
		},
	}
	orphan := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "obsolete-device",
			Labels: map[string]string{deviceNodeLabelKey: "worker-b", deviceIndexLabelKey: "99"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-b",
		},
	}

	client := newTestClient(scheme, node, primary, orphan)

	handler := &trackingHandler{name: "noop"}
	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, []contracts.InventoryHandler{handler})
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

	if err := client.Get(ctx, types.NamespacedName{Name: orphan.Name}, &gpuv1alpha1.GPUDevice{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("orphan device should be deleted, got err=%v", err)
	}

	updated := &gpuv1alpha1.GPUDevice{}
	if err := client.Get(ctx, types.NamespacedName{Name: primary.Name}, updated); err != nil {
		t.Fatalf("failed to get primary device: %v", err)
	}
	if updated.Status.Managed {
		t.Fatal("expected managed flag to be false after reconcile")
	}
	if updated.Labels[deviceIndexLabelKey] != "0" {
		t.Fatalf("expected index label to remain 0, got %s", updated.Labels[deviceIndexLabelKey])
	}

	inventory := &gpuv1alpha1.GPUNodeInventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
		t.Fatalf("inventory missing: %v", err)
	}
	if len(inventory.Status.Hardware.Devices) != 1 {
		t.Fatalf("inventory devices mismatch: %#v", inventory.Status.Hardware)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionManagedDisabled); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected ManagedDisabled=true, got %+v", cond)
	}
	if cond := getCondition(inventory.Status.Conditions, conditionInventoryIncomplete); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InventoryIncomplete=true, got %+v", cond)
	}
}

func TestReconcileCleanupOnMissingNode(t *testing.T) {
	scheme := newTestScheme(t)
	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-c-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-c", deviceIndexLabelKey: "0"},
		},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName: "worker-c",
		},
	}
	inventory := &gpuv1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-c"},
		Spec:       gpuv1alpha1.GPUNodeInventorySpec{NodeName: "worker-c"},
	}

	client := newTestClient(scheme, device, inventory)

	module := defaultModuleSettings()
	reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
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

	if err := client.Get(ctx, types.NamespacedName{Name: device.Name}, &gpuv1alpha1.GPUDevice{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected device to be removed, err=%v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: inventory.Name}, &gpuv1alpha1.GPUNodeInventory{}); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected inventory to be removed, err=%v", err)
	}
}

func TestDeviceApprovalAutoAttachPolicies(t *testing.T) {
	baseModule := defaultModuleSettings()

	tests := []struct {
		name           string
		configure      func(*config.ModuleSettings)
		wantAutoAttach bool
	}{
		{
			name: "manual",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeManual
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: false,
		},
		{
			name: "automatic",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: true,
		},
		{
			name: "selector-match",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "10de"},
				}
			},
			wantAutoAttach: true,
		},
		{
			name: "selector-miss",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "1234"},
				}
			},
			wantAutoAttach: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			module := baseModule
			tt.configure(&module)

			scheme := newTestScheme(t)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auto-node-" + tt.name,
					UID:  types.UID("node-" + tt.name),
					Labels: map[string]string{
						"gpu.deckhouse.io/device.00.vendor": "10de",
						"gpu.deckhouse.io/device.00.device": "1db5",
						"gpu.deckhouse.io/device.00.class":  "0302",
					},
				},
			}

			client := newTestClient(scheme, node)
			reconciler, err := New(testr.New(t), config.ControllerConfig{}, module, nil)
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

			deviceName := buildDeviceName(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"})
			device := &gpuv1alpha1.GPUDevice{}
			if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
				t.Fatalf("expected device, got error: %v", err)
			}
			if device.Status.AutoAttach != tt.wantAutoAttach {
				t.Fatalf("autoAttach mismatch: want %v got %v", tt.wantAutoAttach, device.Status.AutoAttach)
			}
		})
	}
}

func TestMapNodeFeatureToNode(t *testing.T) {
	feature := &nfdv1alpha1.NodeFeature{ObjectMeta: metav1.ObjectMeta{Name: "worker-d"}}
	reqs := mapNodeFeatureToNode(context.Background(), feature)
	if len(reqs) != 1 || reqs[0].Name != "worker-d" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}

	noName := &nfdv1alpha1.NodeFeature{}
	if reqs := mapNodeFeatureToNode(context.Background(), noName); len(reqs) != 0 {
		t.Fatalf("expected empty requests, got %+v", reqs)
	}
}

func getCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := nfdv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add nfd scheme: %v", err)
	}
	return scheme
}

func newTestClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	builder := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.GPUDevice{}, &gpuv1alpha1.GPUNodeInventory{}).
		WithObjects(objs...)

	builder = builder.WithIndex(&gpuv1alpha1.GPUDevice{}, deviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*gpuv1alpha1.GPUDevice)
		if !ok || device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	})

	return builder.Build()
}
