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

package selection

import (
	"testing"

	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestSelectionSyncBasics(t *testing.T) {
	h := NewSelectionSyncHandler(testr.New(t), nil)
	if h.Name() != "selection-sync" {
		t.Fatalf("unexpected handler name: %s", h.Name())
	}

	dev := v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a"},
		Status:     v1alpha1.GPUDeviceStatus{InventoryID: " inv "},
	}
	if key := deviceSortKey(dev); key != "inv" {
		t.Fatalf("unexpected sort key: %q", key)
	}
	dev.Status.InventoryID = ""
	if key := deviceSortKey(dev); key != "dev-a" {
		t.Fatalf("unexpected sort key fallback: %q", key)
	}
}

func TestSelectionSyncUnitsForDevice(t *testing.T) {
	h := NewSelectionSyncHandler(testr.New(t), nil)

	dev := v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{
		Hardware: v1alpha1.GPUDeviceHardware{
			MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "1g.10gb", Count: 2}}},
		},
	}}

	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"}}}
	if got := h.unitsForDevice(dev, pool); got != 0 {
		t.Fatalf("expected 0 without migProfile, got %d", got)
	}

	pool.Spec.Resource.MIGProfile = "2g.20gb"
	pool.Spec.Resource.SlicesPerUnit = 2
	if got := h.unitsForDevice(dev, pool); got != 0 {
		t.Fatalf("expected 0 for missing profile, got %d", got)
	}

	pool.Spec.Resource.MIGProfile = "1g.10gb"
	if got := h.unitsForDevice(dev, pool); got != 4 {
		t.Fatalf("expected 4 (2 profiles * 2 slices), got %d", got)
	}

	pool.Spec.Resource.SlicesPerUnit = 0
	if got := h.unitsForDevice(dev, pool); got != 2 {
		t.Fatalf("expected 2 (profiles count), got %d", got)
	}

	cardPool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	if got := h.unitsForDevice(dev, cardPool); got != 1 {
		t.Fatalf("expected 1 default slice for Card, got %d", got)
	}
	cardPool.Spec.Resource.SlicesPerUnit = 3
	if got := h.unitsForDevice(dev, cardPool); got != 3 {
		t.Fatalf("expected 3 slices for Card, got %d", got)
	}
}

func TestSelectionSyncNeedsAssignmentUpdate(t *testing.T) {
	dev := v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{}}
	if !needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected update when poolRef is nil")
	}

	dev.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "other"}
	if !needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected update when poolRef name differs")
	}

	dev.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"}
	dev.Status.State = v1alpha1.GPUDeviceStatePendingAssignment
	if needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected no update when poolRef matches and device is not Ready")
	}

	dev.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "pool"}
	if !needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected update when poolRef namespace is empty")
	}

	dev.Status.State = v1alpha1.GPUDeviceStateReady
	if !needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected update when device is Ready")
	}

	dev.Status.State = v1alpha1.GPUDeviceStatePendingAssignment
	dev.Status.PoolRef.Namespace = "ns"
	if !needsAssignmentUpdate(dev, "pool", "") {
		t.Fatalf("expected update when pool namespace is empty but ref namespace is set")
	}

	dev.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"}
	if !needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected update when poolRef namespace differs")
	}
}
