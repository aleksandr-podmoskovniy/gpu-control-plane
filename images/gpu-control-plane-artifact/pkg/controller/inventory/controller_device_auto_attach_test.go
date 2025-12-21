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
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

func TestReconcileDeviceAutoAttachAutomatic(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-auto",
			UID:  types.UID("worker-auto"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-auto-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-auto", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{},
	}

	module := defaultModuleSettings()
	module.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	_, _, err = reconciler.deviceSvc().Reconcile(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nil)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if tracker.patches == 0 {
		t.Fatalf("expected status patch to be emitted")
	}
	updated := &v1alpha1.GPUDevice{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: device.Name}, updated); err != nil {
		t.Fatalf("failed to fetch updated device: %v", err)
	}
	if !updated.Status.AutoAttach {
		t.Fatalf("expected autoAttach=true")
	}
}

func TestReconcileDeviceUpdatesProductUUIDAndAutoAttach(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-update-fields",
			UID:  types.UID("node-update-fields"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-update-fields-0-10de-2230",
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:   "worker-update-fields",
			Managed:    true,
			AutoAttach: false,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "Old Product",
				UUID:    "GPU-OLD-UUID",
			},
		},
	}

	base := newTestClient(scheme, node, device)
	client := &delegatingClient{
		Client:       base,
		statusWriter: base.Status(),
	}

	module := defaultModuleSettings()
	module.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{
		Index:   "0",
		Vendor:  "10de",
		Device:  "2230",
		Class:   "0302",
		Product: "New Product",
		MIG:     v1alpha1.GPUMIGConfig{Capable: true, Strategy: v1alpha1.GPUMIGStrategyMixed},
		UUID:    "GPU-NEW-UUID",
	}

	updated, _, err := reconciler.deviceSvc().Reconcile(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nil)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if updated.Status.Hardware.Product != "New Product" {
		t.Fatalf("expected product to be updated, got %s", updated.Status.Hardware.Product)
	}
	if updated.Status.Hardware.UUID != "GPU-NEW-UUID" {
		t.Fatalf("expected uuid to be updated, got %s", updated.Status.Hardware.UUID)
	}
	if updated.Status.Hardware.MIG.Strategy != v1alpha1.GPUMIGStrategyMixed {
		t.Fatalf("expected MIG strategy mixed, got %s", updated.Status.Hardware.MIG.Strategy)
	}
	if !updated.Status.AutoAttach {
		t.Fatal("expected auto attach to be enabled in automatic mode")
	}
}
