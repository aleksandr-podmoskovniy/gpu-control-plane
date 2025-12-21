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

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

func TestReconcileDeviceSetsPCIAddress(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-addr",
			UID:  types.UID("uid-addr"),
		},
	}
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.client = newTestClient(scheme, node)
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(4)

	snapshot := deviceSnapshot{
		Index:      "0",
		Vendor:     "10de",
		Device:     "1db5",
		Class:      "0300",
		PCIAddress: "0000:17:00.0",
		Product:    "NVIDIA-TEST",
		MemoryMiB:  1024,
	}
	device, _, err := rec.deviceSvc().Reconcile(context.Background(), node, snapshot, map[string]string{}, true, rec.fallbackApproval, nil)
	if err != nil {
		t.Fatalf("reconcileDevice returned error: %v", err)
	}
	if device.Status.Hardware.PCI.Address != snapshot.PCIAddress {
		t.Fatalf("expected pci address %s, got %s", snapshot.PCIAddress, device.Status.Hardware.PCI.Address)
	}
}
