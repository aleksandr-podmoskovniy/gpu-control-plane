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
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

func TestSelectionSyncHandlePoolAssignsAndClears(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	max := int32(1)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1, MaxDevicesPerNode: &max},
		},
	}

	devIgnored := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "ignored",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"},
			Labels:      map[string]string{"gpu.deckhouse.io/ignore": "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"},
	}
	devNoNode := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "no-node",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"},
		},
	}
	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-a",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv-b",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStatePendingAssignment,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns"},
		},
	}
	stale := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "stale"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			State:    v1alpha1.GPUDeviceStateAssigned,
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns"},
		},
	}
	staleMismatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "stale-mismatch"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			State:    v1alpha1.GPUDeviceStateAssigned,
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "other"},
		},
	}
	stillAssigned := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "still-assigned",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			State:    v1alpha1.GPUDeviceStatePendingAssignment,
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns"},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(devIgnored, devNoNode, dev1, dev2, stale, staleMismatch, stillAssigned).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	if pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected total capacity to honour maxDevicesPerNode, got %d", pool.Status.Capacity.Total)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("get dev1: %v", err)
	}
	if updated.Status.PoolRef == nil || updated.Status.PoolRef.Name != "pool-a" || updated.Status.PoolRef.Namespace != "ns" {
		t.Fatalf("unexpected poolRef: %+v", updated.Status.PoolRef)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("expected Ready -> PendingAssignment, got %s", updated.Status.State)
	}

	cleared := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "stale"}, cleared); err != nil {
		t.Fatalf("get stale: %v", err)
	}
	if cleared.Status.PoolRef != nil || cleared.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected stale device to be cleared to Ready, got ref=%+v state=%s", cleared.Status.PoolRef, cleared.Status.State)
	}
}

func TestSelectionSyncHandlePoolNodeSelectorFiltersNodesAndSkipsZeroUnits(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"},
		Spec: v1alpha1.GPUPoolSpec{
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}},
			Resource:     v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"},
		},
	}

	nodeMatch := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-match", Labels: map[string]string{"gpu": "true"}}}
	nodeNoMatch := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-nomatch", Labels: map[string]string{"gpu": "false"}}}

	devMatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-match", Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"}},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-match",
			Hardware: v1alpha1.GPUDeviceHardware{MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "1g.10gb", Count: 2}}}},
		},
	}
	devNoMatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-nomatch", Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"}},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-nomatch"},
	}
	devMissingNode := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-missing-node", Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"}},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-missing"},
	}
	devZeroUnits := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-zero", Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool-a"}},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-match",
			Hardware: v1alpha1.GPUDeviceHardware{MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "2g.20gb", Count: 1}}}},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(nodeMatch, nodeNoMatch, devMatch, devNoMatch, devMissingNode, devZeroUnits).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	// Only dev-match contributes (2 profiles).
	if pool.Status.Capacity.Total != 2 {
		t.Fatalf("unexpected capacity total: %d", pool.Status.Capacity.Total)
	}
}
