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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

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

	if err := h.clearDevicePool(context.Background(), "missing", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected missing device to be ignored, got %v", err)
	}

	deviceWithAnnotation := deviceTemplate.DeepCopy()
	deviceWithAnnotation.Name = "dev2"
	deviceWithAnnotation.Annotations = map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"}
	if err := cl.Create(context.Background(), deviceWithAnnotation); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev2", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected assigned annotation to block clearing, got %v", err)
	}

	otherRef := deviceTemplate.DeepCopy()
	otherRef.Name = "dev3"
	otherRef.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "other", Namespace: "ns"}
	if err := cl.Create(context.Background(), otherRef); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev3", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected poolRef mismatch to skip clearing, got %v", err)
	}

	nsMismatch := deviceTemplate.DeepCopy()
	nsMismatch.Name = "dev4"
	nsMismatch.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"}
	if err := cl.Create(context.Background(), nsMismatch); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.clearDevicePool(context.Background(), "dev4", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected namespace mismatch to skip clearing, got %v", err)
	}

	if err := h.clearDevicePool(context.Background(), "dev1", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err != nil {
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
	if err := h.clearDevicePool(context.Background(), "dev5", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("expected NotFound patch to be ignored, got %v", err)
	}

	getErr := selectionGetErrorClient{Client: cl, err: apierrors.NewBadRequest("boom")}
	h = NewSelectionSyncHandler(testr.New(t), getErr)
	if err := h.clearDevicePool(context.Background(), "dev1", "pool", "ns", poolcommon.NamespacedAssignmentAnnotation); err == nil {
		t.Fatalf("expected get error")
	}
}
