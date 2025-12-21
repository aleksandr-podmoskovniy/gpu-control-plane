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

package pool

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/testutil"
)

type selectionStatusPatchErrorWriter struct {
	client.StatusWriter
	err error
}

func (w selectionStatusPatchErrorWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return w.err
}

type selectionStatusPatchErrorClient struct {
	client.Client
	err error
}

func (c selectionStatusPatchErrorClient) Status() client.StatusWriter {
	return selectionStatusPatchErrorWriter{StatusWriter: c.Client.Status(), err: c.err}
}

type selectionGetErrorClient struct {
	client.Client
	err error
}

func (c selectionGetErrorClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}

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
	if needsAssignmentUpdate(dev, "pool", "ns") {
		t.Fatalf("expected legacy poolRef without namespace to be accepted")
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

func TestSelectionSyncAssignDeviceWithRetry(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(&v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev1"}, Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady}}).
		Build()
	h := NewSelectionSyncHandler(testr.New(t), cl)

	if err := h.assignDeviceWithRetry(context.Background(), "missing", "pool", "ns"); err != nil {
		t.Fatalf("expected missing device to be ignored, got %v", err)
	}

	if err := h.assignDeviceWithRetry(context.Background(), "dev1", "pool", "ns"); err != nil {
		t.Fatalf("assignDeviceWithRetry: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.PoolRef == nil || updated.Status.PoolRef.Name != "pool" || updated.Status.PoolRef.Namespace != "ns" {
		t.Fatalf("unexpected poolRef: %+v", updated.Status.PoolRef)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("expected Ready -> PendingAssignment, got %s", updated.Status.State)
	}

	notFoundPatch := selectionStatusPatchErrorClient{
		Client: cl,
		err:    apierrors.NewNotFound(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"}, "dev1"),
	}
	h = NewSelectionSyncHandler(testr.New(t), notFoundPatch)
	if err := h.assignDeviceWithRetry(context.Background(), "dev1", "pool", ""); err != nil {
		t.Fatalf("expected NotFound patch to be ignored, got %v", err)
	}

	getErr := selectionGetErrorClient{Client: cl, err: apierrors.NewBadRequest("boom")}
	h = NewSelectionSyncHandler(testr.New(t), getErr)
	if err := h.assignDeviceWithRetry(context.Background(), "dev1", "pool", "ns"); err == nil {
		t.Fatalf("expected get error")
	}
}

func TestSelectionSyncClearDevicePool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	deviceTemplate := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{},
		},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"},
			State:   v1alpha1.GPUDeviceStateAssigned,
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(deviceTemplate.DeepCopy()).
		Build()
	h := NewSelectionSyncHandler(testr.New(t), cl)

	if err := h.clearDevicePool(context.Background(), "missing", "pool", "ns", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected missing device to be ignored, got %v", err)
	}

	deviceWithAnnotation := deviceTemplate.DeepCopy()
	deviceWithAnnotation.Name = "dev2"
	deviceWithAnnotation.Annotations = map[string]string{NamespacedAssignmentAnnotation: "pool"}
	if err := cl.Create(context.Background(), deviceWithAnnotation); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev2", "pool", "ns", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected assigned annotation to block clearing, got %v", err)
	}

	otherRef := deviceTemplate.DeepCopy()
	otherRef.Name = "dev3"
	otherRef.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "other", Namespace: "ns"}
	if err := cl.Create(context.Background(), otherRef); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev3", "pool", "ns", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected poolRef mismatch to skip clearing, got %v", err)
	}

	nsMismatch := deviceTemplate.DeepCopy()
	nsMismatch.Name = "dev4"
	nsMismatch.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"}
	if err := cl.Create(context.Background(), nsMismatch); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev4", "pool", "ns", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected namespace mismatch to skip clearing, got %v", err)
	}

	if err := h.clearDevicePool(context.Background(), "dev1", "pool", "ns", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("clearDevicePool: %v", err)
	}
	cleared := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, cleared); err != nil {
		t.Fatalf("get: %v", err)
	}
	if cleared.Status.PoolRef != nil || cleared.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected poolRef cleared and state Ready, got ref=%+v state=%s", cleared.Status.PoolRef, cleared.Status.State)
	}

	notFoundPatch := selectionStatusPatchErrorClient{
		Client: cl,
		err:    apierrors.NewNotFound(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"}, "dev1"),
	}
	h = NewSelectionSyncHandler(testr.New(t), notFoundPatch)
	dev5 := deviceTemplate.DeepCopy()
	dev5.Name = "dev5"
	if err := cl.Create(context.Background(), dev5); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev5", "pool", "ns", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected NotFound patch to be ignored, got %v", err)
	}

	getErr := selectionGetErrorClient{Client: cl, err: apierrors.NewBadRequest("boom")}
	h = NewSelectionSyncHandler(testr.New(t), getErr)
	if err := h.clearDevicePool(context.Background(), "dev1", "pool", "ns", NamespacedAssignmentAnnotation); err == nil {
		t.Fatalf("expected get error")
	}
}

func TestSelectionSyncHandlePoolErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}

	noIndexes := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewSelectionSyncHandler(testr.New(t), noIndexes)
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected list error without indexes")
	}

	// Index only assignment field, but not poolRef field.
	partial := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDeviceNamespacedAssignmentField, func(obj client.Object) []string {
			dev, ok := obj.(*v1alpha1.GPUDevice)
			if !ok {
				return nil
			}
			// Use annotation directly (normal behaviour).
			if dev.Annotations == nil {
				return nil
			}
			if v := dev.Annotations[NamespacedAssignmentAnnotation]; v != "" {
				return []string{v}
			}
			return nil
		}).
		Build()
	h = NewSelectionSyncHandler(testr.New(t), partial)
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected poolRef list error without index")
	}

	invalid := pool.DeepCopy()
	invalid.Spec.NodeSelector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "a", Operator: "BadOp", Values: []string{"b"}}},
	}
	full := testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	h = NewSelectionSyncHandler(testr.New(t), full)
	if _, err := h.HandlePool(context.Background(), invalid); err == nil {
		t.Fatalf("expected invalid nodeSelector error")
	}

	nodeGetErr := selectionGetErrorClient{Client: full, err: errors.New("boom")}
	h = NewSelectionSyncHandler(testr.New(t), nodeGetErr)
	filtered := pool.DeepCopy()
	filtered.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}}
	dev := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev", Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool"}}, Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"}}
	if err := full.Create(context.Background(), dev); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := h.HandlePool(context.Background(), filtered); err == nil {
		t.Fatalf("expected node get error")
	}
}

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
			Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"},
			Labels:      map[string]string{"gpu.deckhouse.io/ignore": "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"},
	}
	devNoNode := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "no-node",
			Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"},
		},
	}
	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"},
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
			Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"},
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
			Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			State:    v1alpha1.GPUDeviceStatePendingAssignment,
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns"},
		},
	}

	cl := testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
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
		ObjectMeta: metav1.ObjectMeta{Name: "dev-match", Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"}},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-match",
			Hardware: v1alpha1.GPUDeviceHardware{MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "1g.10gb", Count: 2}}}},
		},
	}
	devNoMatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-nomatch", Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"}},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-nomatch"},
	}
	devMissingNode := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-missing-node", Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"}},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: "node-missing"},
	}
	devZeroUnits := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-zero", Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool-a"}},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-match",
			Hardware: v1alpha1.GPUDeviceHardware{MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "2g.20gb", Count: 1}}}},
		},
	}

	cl := testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
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
