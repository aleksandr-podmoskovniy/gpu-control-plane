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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcileDeviceNoStatusPatchWhenUnchanged(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-nochange",
			UID:  types.UID("nochange"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-nochange-0-10de-1db5",
			Labels: map[string]string{deviceNodeLabelKey: "worker-nochange", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "worker-nochange",
			InventoryID: buildInventoryID("worker-nochange", deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5"}),
			Managed:     true,
			Hardware: v1alpha1.GPUDeviceHardware{
				PCI:     v1alpha1.PCIAddress{Vendor: "10de", Device: "1db5", Class: "0302"},
				UUID:    "GPU-NOCHANGE-UUID",
				Product: "Existing",
				MIG:     v1alpha1.GPUMIGConfig{},
			},
		},
	}

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{
		Index:   "0",
		Vendor:  "10de",
		Device:  "1db5",
		Class:   "0302",
		UUID:    "GPU-NOCHANGE-UUID",
		Product: "Existing",
	}
	_, _, err = reconciler.deviceSvc().Reconcile(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nil)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if tracker.patches != 0 {
		t.Fatalf("expected no status patch when nothing changes")
	}
}

func TestReconcileDeviceSkipsStatusPatchWhenUnchanged(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-unchanged",
			UID:  types.UID("worker-unchanged"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	deviceName := buildDeviceName(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"})
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deviceName,
			Labels: map[string]string{deviceNodeLabelKey: node.Name, deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    node.Name,
			InventoryID: buildInventoryID(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}),
			Managed:     true,
			Hardware: v1alpha1.GPUDeviceHardware{
				Product: "NVIDIA TEST",
				UUID:    "GPU-UNCHANGED-UUID",
				PCI:     v1alpha1.PCIAddress{Vendor: "10de", Device: "1db5", Class: "0302"},
				MIG:     v1alpha1.GPUMIGConfig{},
			},
			AutoAttach: false,
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
		t.Fatalf("set owner reference: %v", err)
	}

	baseClient := newTestClient(scheme, node, device)
	tracker := &trackingStatusWriter{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: tracker}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{
		Index:   "0",
		Vendor:  "10de",
		Device:  "1db5",
		Class:   "0302",
		Product: "NVIDIA TEST",
		UUID:    "GPU-UNCHANGED-UUID",
	}

	_, result, err := reconciler.deviceSvc().Reconcile(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nil)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}
	if tracker.patches != 0 {
		t.Fatalf("expected no status patch, got %d patches", tracker.patches)
	}
}
