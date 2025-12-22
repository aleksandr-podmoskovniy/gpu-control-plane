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
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestDeviceServiceReconcileUpdatesHardware(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)

	node := newTestNode("node-hw-update")
	existing := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   invstate.BuildDeviceName(node.Name, newTestSnapshot()),
			Labels: map[string]string{invstate.DeviceNodeLabelKey: node.Name, invstate.DeviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: node.Name,
			Hardware: v1alpha1.GPUDeviceHardware{
				PCI: v1alpha1.PCIAddress{Vendor: "10de", Device: "1db4", Class: "0300"},
			},
		},
	}

	base := newTestClient(t, scheme, node, existing)
	approval := invstate.DeviceApprovalPolicy{Mode: moduleconfig.DeviceApprovalModeAutomatic}

	snapshot := newTestSnapshot()
	snapshot.Device = "1db4"
	snapshot.Class = "0300"
	snapshot.PCIAddress = "0000:65:00.0"
	snapshot.Product = "A100"
	snapshot.UUID = "GPU-AAA"
	snapshot.MIG = v1alpha1.GPUMIGConfig{
		Capable:           true,
		Strategy:          v1alpha1.GPUMIGStrategySingle,
		ProfilesSupported: []string{"1g.10gb"},
		Types: []v1alpha1.GPUMIGTypeCapacity{
			{Name: "1g.10gb", Count: 7},
		},
	}

	svc := NewDeviceService(base, scheme, nil, nil)
	updated, _, err := svc.Reconcile(ctx, node, snapshot, map[string]string{}, true, approval, nil)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	hw := updated.Status.Hardware
	if hw.Product != snapshot.Product || hw.UUID != snapshot.UUID {
		t.Fatalf("expected product/uuid updated: %+v", hw)
	}
	if hw.PCI.Address != snapshot.PCIAddress {
		t.Fatalf("expected pci address %s, got %s", snapshot.PCIAddress, hw.PCI.Address)
	}
	if !equality.Semantic.DeepEqual(hw.MIG, snapshot.MIG) {
		t.Fatalf("expected MIG config updated, got %+v", hw.MIG)
	}
	if !updated.Status.AutoAttach {
		t.Fatalf("expected autoAttach=true")
	}
}
