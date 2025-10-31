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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestDeviceInventorySyncWritesStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = gpuv1alpha1.AddToScheme(scheme)

	inventory := &gpuv1alpha1.GPUNodeInventory{
		TypeMeta:   metav1.TypeMeta{Kind: "GPUNodeInventory", APIVersion: gpuv1alpha1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&gpuv1alpha1.GPUNodeInventory{}).WithObjects(inventory).Build()
	h := NewDeviceInventorySync(testr.New(t), fakeClient)

	device := &gpuv1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-a"},
		Status: gpuv1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "node-a-0000",
			State:       gpuv1alpha1.GPUDeviceStateReserved,
			AutoAttach:  true,
		},
	}

	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &gpuv1alpha1.GPUNodeInventory{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "node-a"}, updated); err != nil {
		t.Fatalf("expected inventory: %v", err)
	}

	if len(updated.Status.Hardware.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(updated.Status.Hardware.Devices))
	}

	d := updated.Status.Hardware.Devices[0]
	if d.InventoryID != "node-a-0000" || d.State != gpuv1alpha1.GPUDeviceStateReserved || !d.AutoAttach {
		t.Fatalf("device status not synced: %+v", d)
	}
}
