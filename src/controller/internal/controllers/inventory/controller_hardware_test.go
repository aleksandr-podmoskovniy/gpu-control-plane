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

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestReconcileDeviceUpdatesExtendedHardware(t *testing.T) {
	scheme := newTestScheme(t)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-extended"}}
	existing := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-extended-0-10de-1db4",
			Labels: map[string]string{deviceNodeLabelKey: node.Name, deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: node.Name,
			Hardware: v1alpha1.GPUDeviceHardware{PCI: v1alpha1.PCIAddress{Vendor: "10de", Device: "1db4", Class: "0300"}},
		},
	}

	client := newTestClient(scheme, node, existing)
	rec := &Reconciler{
		client:   client,
		scheme:   scheme,
		log:      testr.New(t),
		recorder: record.NewFakeRecorder(32),
	}

	snap := deviceSnapshot{
		Index:        "0",
		Vendor:       "10de",
		Device:       "1db4",
		Class:        "0300",
		PCIAddress:   "0000:65:00.0",
		Product:      "A100",
		MemoryMiB:    40960,
		ComputeMajor: 8,
		ComputeMinor: 0,
		NUMANode:     ptrInt32(1),
		PowerLimitMW: ptrInt32(250),
		SMCount:      ptrInt32(108),
		MemBandwidth: ptrInt32(1555),
		PCIEGen:      ptrInt32(4),
		PCIELinkWid:  ptrInt32(16),
		Board:        "board-id",
		Family:       "ampere",
		Serial:       "serial-1",
		PState:       "P0",
		DisplayMode:  "Enabled",
		Precision:    []string{"fp16"},
	}

	approval := DeviceApprovalPolicy{}
	if _, _, err := rec.reconcileDevice(context.Background(), node, snap, node.Labels, true, approval, nodeTelemetry{}, nodeDetection{}); err != nil {
		t.Fatalf("reconcileDevice returned error: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: existing.Name}, updated); err != nil {
		t.Fatalf("failed to get updated device: %v", err)
	}

	hw := updated.Status.Hardware
	if !int32PtrEqual(hw.NUMANode, snap.NUMANode) || !int32PtrEqual(hw.PowerLimitMilliWatt, snap.PowerLimitMW) || !int32PtrEqual(hw.SMCount, snap.SMCount) || !int32PtrEqual(hw.MemoryBandwidthMiB, snap.MemBandwidth) {
		t.Fatalf("hardware pointers not updated: %+v", hw)
	}
	if !int32PtrEqual(hw.PCIE.Generation, snap.PCIEGen) || !int32PtrEqual(hw.PCIE.Width, snap.PCIELinkWid) {
		t.Fatalf("pcie not updated: %+v", hw.PCIE)
	}
	if hw.Board != snap.Board || hw.Family != snap.Family || hw.Serial != snap.Serial || hw.PState != snap.PState || hw.DisplayMode != snap.DisplayMode {
		t.Fatalf("string fields not updated: %+v", hw)
	}
}

func TestInt32PtrEqual(t *testing.T) {
	if !int32PtrEqual(nil, nil) {
		t.Fatalf("expected nil == nil")
	}
	if int32PtrEqual(ptrInt32(1), nil) || int32PtrEqual(nil, ptrInt32(1)) {
		t.Fatalf("expected mismatch when only one is nil")
	}
	if !int32PtrEqual(ptrInt32(2), ptrInt32(2)) || int32PtrEqual(ptrInt32(2), ptrInt32(3)) {
		t.Fatalf("int32PtrEqual comparison failed")
	}
}
