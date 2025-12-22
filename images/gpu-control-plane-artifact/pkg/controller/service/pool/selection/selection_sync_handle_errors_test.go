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
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

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
			if v := dev.Annotations[poolcommon.NamespacedAssignmentAnnotation]; v != "" {
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
	full := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	h = NewSelectionSyncHandler(testr.New(t), full)
	if _, err := h.HandlePool(context.Background(), invalid); err == nil {
		t.Fatalf("expected invalid nodeSelector error")
	}

	nodeGetErr := selectionGetErrorClient{Client: full, err: errors.New("boom")}
	h = NewSelectionSyncHandler(testr.New(t), nodeGetErr)
	filtered := pool.DeepCopy()
	filtered.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}}
	dev := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev", Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"}}, Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"}}
	if err := full.Create(context.Background(), dev); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := h.HandlePool(context.Background(), filtered); err == nil {
		t.Fatalf("expected node get error")
	}
}
