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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/testutil"
)

func TestSelectionSyncHandlePoolReturnsClearErrorAndCoversClusterAssignment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	stale := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev1"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	base := testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(stale).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), selectionStatusPatchErrorClient{Client: base, err: errors.New("patch error")})
	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterGPUPool"},
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
	}

	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected clearDevicePool patch error")
	}
}

func TestSelectionSyncHandlePoolReturnsAssignError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{NamespacedAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:    v1alpha1.GPUDeviceStateReady,
			NodeName: "node1",
		},
	}

	base := testutil.WithPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), selectionStatusPatchErrorClient{Client: base, err: errors.New("patch error")})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}

	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected assignDeviceWithRetry patch error")
	}
}

func TestSelectionSyncClearDevicePoolSkipsWhenPoolNamespaceEmptyButRefNamespacePresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev1"},
		Status: v1alpha1.GPUDeviceStatus{
			State:   v1alpha1.GPUDeviceStateAssigned,
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(dev).
		Build()

	h := NewSelectionSyncHandler(testr.New(t), cl)
	if err := h.clearDevicePool(context.Background(), "dev1", "pool", "", NamespacedAssignmentAnnotation); err != nil {
		t.Fatalf("clearDevicePool: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Status.PoolRef == nil || updated.Status.PoolRef.Namespace != "ns" {
		t.Fatalf("expected poolRef to remain unchanged, got %+v", updated.Status.PoolRef)
	}
}
