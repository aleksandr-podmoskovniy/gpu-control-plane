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
	"fmt"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestSelectionSyncCoversPodFiltersAndAssignments(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	devAssigned := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateReserved,
			AutoAttach:  true,
		},
	}
	devNeedsUpdate := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev2",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateReady,
		},
	}
	devToClear := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev3"},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev3",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateAssigned,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	resourceName := corev1.ResourceName("gpu.deckhouse.io/pool")
	pods := []client.Object{
		&corev1.Pod{ // no node
			ObjectMeta: metav1.ObjectMeta{Name: "nonode"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
					},
				}},
			},
		},
		&corev1.Pod{ // succeeded
			ObjectMeta: metav1.ObjectMeta{Name: "done"},
			Spec: corev1.PodSpec{
				NodeName: "node1",
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
		&corev1.Pod{ // running, contributes usage
			ObjectMeta: metav1.ObjectMeta{Name: "using"},
			Spec: corev1.PodSpec{
				NodeName: "node1",
				Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{resourceName: resource.MustParse("2")},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	objects := []client.Object{devAssigned, devNeedsUpdate, devToClear}
	objects = append(objects, pods...)

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(objects...).
		Build()

	handler := NewSelectionSyncHandler(testr.New(t), cl)
	if handler.Name() == "" {
		t.Fatalf("expected handler name")
	}
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool failed: %v", err)
	}

	if pool.Status.Capacity.Total == 0 {
		t.Fatalf("expected capacity to be calculated, got %+v", pool.Status.Capacity)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev2"}, updated); err != nil {
		t.Fatalf("get dev2: %v", err)
	}
	if updated.Status.State != v1alpha1.GPUDeviceStatePendingAssignment {
		t.Fatalf("expected dev2 state to change, got %s", updated.Status.State)
	}

	cleared := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev3"}, cleared); err != nil {
		t.Fatalf("get dev3: %v", err)
	}
	if cleared.Status.PoolRef != nil {
		t.Fatalf("expected dev3 pool ref cleared")
	}
}

func TestSelectionSyncListErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	devErr := apierrors.NewInternalError(fmt.Errorf("devices list failed"))
	cl := &listErrorClient{Client: withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build(), errs: map[string]error{fmt.Sprintf("%T", &v1alpha1.GPUDeviceList{}): devErr}}
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	if _, err := handler.HandlePool(context.Background(), &v1alpha1.GPUPool{}); !errors.Is(err, devErr) {
		t.Fatalf("expected devices list error, got %v", err)
	}

	// pod list error path should be ignored and still compute from empty usage.
	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev",
			NodeName:    "node",
			State:       v1alpha1.GPUDeviceStateAssigned,
		},
	}
	base := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()
	podListErr := apierrors.NewInternalError(fmt.Errorf("pods list failed"))
	cl = &listErrorClient{Client: base, errs: map[string]error{fmt.Sprintf("%T", &corev1.PodList{}): podListErr}}
	handler = NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("expected pod list error to be ignored, got %v", err)
	}
	if pool.Status.Capacity.Total == 0 {
		t.Fatalf("expected capacity computed even without pods")
	}
}

func TestSelectionSyncInvalidNodeSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		Spec: v1alpha1.GPUPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key: "invalid[",
				}},
			},
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected invalid selector error")
	}
}

func TestSelectionSyncHelpersCoverage(t *testing.T) {
	handler := NewSelectionSyncHandler(testr.New(t), nil)
	dev := v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "id",
			Hardware: v1alpha1.GPUDeviceHardware{
				MIG: v1alpha1.GPUMIGConfig{Types: []v1alpha1.GPUMIGTypeCapacity{{Name: "1g", Count: 2}}},
			},
		},
	}
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{
		Unit:          "MIG",
		MIGProfile:    "1g",
		SlicesPerUnit: 3,
	}}}
	if units := handler.unitsForDevice(dev, pool); units != 6 {
		t.Fatalf("expected MIG units 6, got %d", units)
	}
	pool.Spec.Resource.Unit = "Card"
	pool.Spec.Resource.SlicesPerUnit = 4
	if units := handler.unitsForDevice(dev, pool); units != 4 {
		t.Fatalf("expected Card units 4, got %d", units)
	}
	pool.Spec.Resource.SlicesPerUnit = 0
	if units := handler.unitsForDevice(dev, pool); units != 1 {
		t.Fatalf("expected default units 1, got %d", units)
	}
	pool.Spec.Resource.Unit = "MIG"
	pool.Spec.Resource.MIGProfile = "1g"
	pool.Spec.Resource.SlicesPerUnit = 0
	if units := handler.unitsForDevice(dev, pool); units != 2 {
		t.Fatalf("expected MIG units 2, got %d", units)
	}
	pool.Spec.Resource.MIGProfile = "missing"
	if units := handler.unitsForDevice(dev, pool); units != 0 {
		t.Fatalf("expected zero units for missing MIG profile, got %d", units)
	}

	if needsAssignmentUpdate(v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"}, State: v1alpha1.GPUDeviceStateReserved}}, "pool", "") {
		t.Fatalf("expected assigned reserved device not to need update")
	}
}

func TestSelectionSyncNeedsAssignmentAndUnitsBranches(t *testing.T) {
	dev := v1alpha1.GPUDevice{}
	if !needsAssignmentUpdate(dev, "p", "") {
		t.Fatalf("missing poolref should need update")
	}
	dev.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "other"}
	if !needsAssignmentUpdate(dev, "p", "") {
		t.Fatalf("different poolref should need update")
	}
	dev.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "p"}
	dev.Status.State = v1alpha1.GPUDeviceStateReady
	if !needsAssignmentUpdate(dev, "p", "") {
		t.Fatalf("ready device should need update")
	}
	dev.Status.State = v1alpha1.GPUDeviceStateAssigned
	if needsAssignmentUpdate(dev, "p", "") {
		t.Fatalf("assigned device should not need update")
	}

	h := NewSelectionSyncHandler(testr.New(t), nil)
	pool := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"}}}
	if units := h.unitsForDevice(v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{Hardware: v1alpha1.GPUDeviceHardware{MIG: v1alpha1.GPUMIGConfig{}}}}, pool); units != 0 {
		t.Fatalf("missing MIG profile should yield zero units")
	}
	pool.Spec.Resource.Unit = "Card"
	pool.Spec.Resource.SlicesPerUnit = 2
	if units := h.unitsForDevice(v1alpha1.GPUDevice{}, pool); units != 2 {
		t.Fatalf("card slices per unit should apply, got %d", units)
	}
}

func TestSelectionSyncNodeGetErrorWithSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateAssigned,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()
	handler := NewSelectionSyncHandler(testr.New(t), &failingGetClient{Client: base})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "gpu"}},
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected node get error with selector")
	}
}

func TestSelectionSyncNodeSelectorHappyPath(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"role": "gpu"}}}
	node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}}

	dev1 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev1",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateAssigned,
		},
	}
	dev2 := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev2",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "dev2",
			NodeName:    "node2",
			State:       v1alpha1.GPUDeviceStateAssigned,
		},
	}
	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(node1, node2, dev1, dev2).
		Build()

	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource:     v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "gpu"}},
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("expected selector reconcile success, got %v", err)
	}
	if pool.Status.Capacity.Total != 1 {
		t.Fatalf("expected capacity only from eligible nodes, got %+v", pool.Status.Capacity)
	}

	updated1 := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated1); err != nil {
		t.Fatalf("get dev1: %v", err)
	}
	if updated1.Status.PoolRef == nil || updated1.Status.PoolRef.Name != "pool" || updated1.Status.State != v1alpha1.GPUDeviceStateAssigned {
		t.Fatalf("expected dev1 assigned to pool, got %+v", updated1.Status)
	}

	updated2 := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev2"}, updated2); err != nil {
		t.Fatalf("get dev2: %v", err)
	}
	if updated2.Status.PoolRef != nil {
		t.Fatalf("expected dev2 to stay unassigned due to nodeSelector, got %+v", updated2.Status)
	}
}

func TestAssignDeviceWithRetryNotFoundAndPatchNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Status:     v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateAssigned},
	}

	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()

	handler := NewSelectionSyncHandler(testr.New(t), base)
	if err := handler.assignDeviceWithRetry(context.Background(), "missing", "pool", ""); err != nil {
		t.Fatalf("expected notfound to be ignored: %v", err)
	}

	statusClient := &statusErrorClient{Client: base, err: apierrors.NewNotFound(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev")}
	handler = NewSelectionSyncHandler(testr.New(t), statusClient)
	if err := handler.assignDeviceWithRetry(context.Background(), "dev", "pool", ""); err != nil {
		t.Fatalf("expected patch notfound to be ignored, got %v", err)
	}

	statusClient = &statusErrorClient{Client: base, err: apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev", nil)}
	handler = NewSelectionSyncHandler(testr.New(t), statusClient)
	if err := handler.assignDeviceWithRetry(context.Background(), "dev", "pool", ""); err == nil {
		t.Fatalf("expected conflict to propagate")
	}

	getErrClient := &failingGetClient{Client: base}
	handler = NewSelectionSyncHandler(testr.New(t), getErrClient)
	if err := handler.assignDeviceWithRetry(context.Background(), "dev", "pool", ""); err == nil {
		t.Fatalf("expected get error to propagate")
	}
}

func TestClearDevicePoolBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()
	handler := NewSelectionSyncHandler(testr.New(t), base)
	// annotation match short-circuits
	if err := handler.clearDevicePool(context.Background(), "dev", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected annotation match to short-circuit: %v", err)
	}

	// successful patch path sets state to Ready
	successDev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-success"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateReserved,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	successBase := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(successDev).
		Build()
	handler = NewSelectionSyncHandler(testr.New(t), successBase)
	if err := handler.clearDevicePool(context.Background(), "dev-success", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected clear success: %v", err)
	}
	reloaded := &v1alpha1.GPUDevice{}
	if err := successBase.Get(context.Background(), client.ObjectKey{Name: "dev-success"}, reloaded); err != nil {
		t.Fatalf("reload device: %v", err)
	}
	if reloaded.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected state reset to Ready, got %s", reloaded.Status.State)
	}

	errClient := &statusErrorClient{Client: successBase, err: apierrors.NewNotFound(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev-success")}
	handler = NewSelectionSyncHandler(testr.New(t), errClient)
	if err := handler.clearDevicePool(context.Background(), "dev-success", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected status notfound to be ignored, got %v", err)
	}

	pendingDev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-pending"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStatePendingAssignment,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	pendingBase := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(pendingDev).
		Build()
	handler = NewSelectionSyncHandler(testr.New(t), pendingBase)
	if err := handler.clearDevicePool(context.Background(), "dev-pending", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected pending device to clear: %v", err)
	}
	pendingReload := &v1alpha1.GPUDevice{}
	_ = pendingBase.Get(context.Background(), client.ObjectKey{Name: "dev-pending"}, pendingReload)
	if pendingReload.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected pending device to become Ready, got %s", pendingReload.Status.State)
	}

	// PoolRef pointing to another pool should no-op.
	otherPoolDev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-other"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "other"},
		},
	}
	otherBase := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(otherPoolDev).
		Build()
	handler = NewSelectionSyncHandler(testr.New(t), otherBase)
	if err := handler.clearDevicePool(context.Background(), "dev-other", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected other pool ref to short-circuit: %v", err)
	}

	// get not found path is ignored
	notFoundClient := &failingGetClient{Client: base, notFound: true}
	handler = NewSelectionSyncHandler(testr.New(t), notFoundClient)
	if err := handler.clearDevicePool(context.Background(), "absent", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected get notfound to be ignored, got %v", err)
	}
	badClient := &failingGetClient{Client: base}
	handler = NewSelectionSyncHandler(testr.New(t), badClient)
	if err := handler.clearDevicePool(context.Background(), "dev", "pool", "", assignmentAnnotation); err == nil {
		t.Fatalf("expected get error to propagate")
	}

	conflictDev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-conflict",
			Annotations: map[string]string{},
		},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
			State:   v1alpha1.GPUDeviceStateAssigned,
		},
	}
	conflictBase := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(conflictDev).
		Build()
	conflictClient := &statusErrorClient{Client: conflictBase, err: apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev-conflict", nil)}
	handler = NewSelectionSyncHandler(testr.New(t), conflictClient)
	if err := handler.clearDevicePool(context.Background(), "dev-conflict", "pool", "", assignmentAnnotation); err == nil {
		t.Fatalf("expected conflict to propagate")
	}

	// poolRef nil short-circuit
	other := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "other"}}
	base = fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(other).
		Build()
	handler = NewSelectionSyncHandler(testr.New(t), base)
	if err := handler.clearDevicePool(context.Background(), "other", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected nil poolref to short-circuit: %v", err)
	}

	// poolRef different name short-circuit
	other.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "x"}
	_ = base.Status().Patch(context.Background(), other, client.MergeFrom(other.DeepCopy()))
	if err := handler.clearDevicePool(context.Background(), "other", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected different poolref to short-circuit: %v", err)
	}
}

func TestSelectionSyncHandlesFallbackKeysAndIgnoresPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev-empty",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "",
			NodeName:    "node1",
			State:       v1alpha1.GPUDeviceStateAssigned,
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "using", Namespace: "ns"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceName("gpu.deckhouse.io/pool"): resource.MustParse("1")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev, pod).
		Build()

	handler := NewSelectionSyncHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 2}},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool failed: %v", err)
	}
	if pool.Status.Capacity.Total != 2 {
		t.Fatalf("expected capacity without pod-driven usage, got %+v", pool.Status.Capacity)
	}
}

func TestSelectionSyncPropagatesAssignAndClearErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	t.Run("clearDevicePool error bubbles up", func(t *testing.T) {
		dev := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-clear"},
			Status: v1alpha1.GPUDeviceStatus{
				PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
				State:    v1alpha1.GPUDeviceStateAssigned,
				NodeName: "node1",
			},
		}
		base := withPoolDeviceIndexes(fake.NewClientBuilder().
			WithScheme(scheme)).
			WithStatusSubresource(&v1alpha1.GPUDevice{}).
			WithObjects(dev).
			Build()
		conflict := apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev-clear", nil)
		cl := &selectiveStatusErrorClient{Client: base, errs: map[string]error{"dev-clear": conflict}}

		handler := NewSelectionSyncHandler(testr.New(t), cl)
		_, err := handler.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}})
		if !apierrors.IsConflict(err) {
			t.Fatalf("expected conflict from clearDevicePool, got %v", err)
		}
	})

	t.Run("assignDeviceWithRetry error bubbles up", func(t *testing.T) {
		dev := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dev-assign",
				Annotations: map[string]string{assignmentAnnotation: "pool"},
			},
			Status: v1alpha1.GPUDeviceStatus{
				InventoryID: "gpu1",
				NodeName:    "node1",
				State:       v1alpha1.GPUDeviceStateReady,
			},
		}
		base := withPoolDeviceIndexes(fake.NewClientBuilder().
			WithScheme(scheme)).
			WithStatusSubresource(&v1alpha1.GPUDevice{}).
			WithObjects(dev).
			Build()
		conflict := apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev-assign", nil)
		cl := &selectiveStatusErrorClient{Client: base, errs: map[string]error{"dev-assign": conflict}}

		handler := NewSelectionSyncHandler(testr.New(t), cl)
		_, err := handler.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}})
		if !apierrors.IsConflict(err) {
			t.Fatalf("expected conflict from assignDeviceWithRetry, got %v", err)
		}
	})
}

func TestClearDevicePoolPatchNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-missing"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()
	cl := &selectiveStatusErrorClient{Client: base, errs: map[string]error{
		"dev-missing": apierrors.NewNotFound(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), "dev-missing"),
	}}

	handler := NewSelectionSyncHandler(testr.New(t), cl)
	if err := handler.clearDevicePool(context.Background(), "dev-missing", "pool", "", assignmentAnnotation); err != nil {
		t.Fatalf("expected notfound from patch to be ignored, got %v", err)
	}
}
