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
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestReconcileDeviceUpdatesHardware(t *testing.T) {
	scheme := newTestScheme(t)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-extended", UID: types.UID("node-extended")}}
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
		Index:      "0",
		Vendor:     "10de",
		Device:     "1db4",
		Class:      "0300",
		PCIAddress: "0000:65:00.0",
		Product:    "A100",
		UUID:       "GPU-AAA",
		MIG: v1alpha1.GPUMIGConfig{
			Capable:           true,
			Strategy:          v1alpha1.GPUMIGStrategySingle,
			ProfilesSupported: []string{"1g.10gb"},
			Types: []v1alpha1.GPUMIGTypeCapacity{
				{Name: "1g.10gb", Count: 7},
			},
		},
	}

	approval := DeviceApprovalPolicy{}
	if _, _, err := rec.deviceSvc().Reconcile(context.Background(), node, snap, node.Labels, true, approval, nodeDetection{}); err != nil {
		t.Fatalf("reconcileDevice returned error: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: existing.Name}, updated); err != nil {
		t.Fatalf("failed to get updated device: %v", err)
	}

	hw := updated.Status.Hardware
	if hw.Product != snap.Product || hw.UUID != snap.UUID {
		t.Fatalf("expected product/uuid updated: %+v", hw)
	}
	if hw.PCI.Address != snap.PCIAddress {
		t.Fatalf("expected pci address %s, got %s", snap.PCIAddress, hw.PCI.Address)
	}
	if !equality.Semantic.DeepEqual(hw.MIG, snap.MIG) {
		t.Fatalf("expected MIG config updated, got %+v", hw.MIG)
	}
}
