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

package indexer

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

type capturingFieldIndexer struct {
	field     string
	extractor client.IndexerFunc
	calls     int
	err       error
}

func (f *capturingFieldIndexer) IndexField(_ context.Context, _ client.Object, field string, extractFunc client.IndexerFunc) error {
	f.calls++
	f.field = field
	f.extractor = extractFunc
	return f.err
}

func TestIndexGPUDeviceByNode(t *testing.T) {
	idx := &capturingFieldIndexer{}

	if err := IndexGPUDeviceByNode(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	if idx.calls != 1 || idx.field != GPUDeviceNodeField {
		t.Fatalf("expected index call for %s, got %d calls field %q", GPUDeviceNodeField, idx.calls, idx.field)
	}

	device := &v1alpha1.GPUDevice{}
	device.Status.NodeName = "node-a"
	values := idx.extractor(device)
	if len(values) != 1 || values[0] != "node-a" {
		t.Fatalf("expected node name indexed, got %+v", values)
	}

	// empty node name -> no index
	device.Status.NodeName = ""
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty node name, got %+v", got)
	}

	// unrelated object should be ignored
	pod := &corev1.Pod{}
	if got := idx.extractor(pod); got != nil {
		t.Fatalf("expected nil for non-GPUDevice object, got %+v", got)
	}
}

func TestIndexGPUDeviceByPoolRefName(t *testing.T) {
	idx := &capturingFieldIndexer{}

	if err := IndexGPUDeviceByPoolRefName(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	if idx.calls != 1 || idx.field != GPUDevicePoolRefNameField {
		t.Fatalf("expected index call for %s, got %d calls field %q", GPUDevicePoolRefNameField, idx.calls, idx.field)
	}

	device := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool-a"}}}
	if got := idx.extractor(device); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected pool name indexed, got %+v", got)
	}

	device.Status.PoolRef = nil
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty pool ref, got %+v", got)
	}

	// unrelated object should be ignored
	pod := &corev1.Pod{}
	if got := idx.extractor(pod); got != nil {
		t.Fatalf("expected nil for non-GPUDevice object, got %+v", got)
	}
}

func TestIndexGPUDeviceByNamespacedAssignment(t *testing.T) {
	idx := &capturingFieldIndexer{}

	if err := IndexGPUDeviceByNamespacedAssignment(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	if idx.calls != 1 || idx.field != GPUDeviceNamespacedAssignmentField {
		t.Fatalf("expected index call for %s, got %d calls field %q", GPUDeviceNamespacedAssignmentField, idx.calls, idx.field)
	}

	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"gpu.deckhouse.io/assignment": "pool-a"}}}
	if got := idx.extractor(device); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected assignment indexed, got %+v", got)
	}

	device.Annotations = nil
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty annotations, got %+v", got)
	}
}

func TestIndexGPUDeviceByClusterAssignment(t *testing.T) {
	idx := &capturingFieldIndexer{}

	if err := IndexGPUDeviceByClusterAssignment(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	if idx.calls != 1 || idx.field != GPUDeviceClusterAssignmentField {
		t.Fatalf("expected index call for %s, got %d calls field %q", GPUDeviceClusterAssignmentField, idx.calls, idx.field)
	}

	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"cluster.gpu.deckhouse.io/assignment": "pool-a"}}}
	if got := idx.extractor(device); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected assignment indexed, got %+v", got)
	}

	device.Annotations = nil
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty annotations, got %+v", got)
	}
}

func TestIndexGPUPoolByName(t *testing.T) {
	idx := &capturingFieldIndexer{}

	if err := IndexGPUPoolByName(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	if idx.calls != 1 || idx.field != GPUPoolNameField {
		t.Fatalf("expected index call for %s, got %d calls field %q", GPUPoolNameField, idx.calls, idx.field)
	}

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	if got := idx.extractor(pool); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected pool name indexed, got %+v", got)
	}

	pool.Name = ""
	if got := idx.extractor(pool); got != nil {
		t.Fatalf("expected nil for empty pool name, got %+v", got)
	}

	if got := idx.extractor(&corev1.Pod{}); got != nil {
		t.Fatalf("expected nil for non-GPUPool object, got %+v", got)
	}
}

func TestIndexersHandleNilIndexerAndPropagateErrors(t *testing.T) {
	if err := IndexGPUDeviceByNode(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for nil indexer, got %v", err)
	}
	if err := IndexGPUDeviceByPoolRefName(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for nil indexer, got %v", err)
	}
	if err := IndexGPUDeviceByNamespacedAssignment(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for nil indexer, got %v", err)
	}
	if err := IndexGPUDeviceByClusterAssignment(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for nil indexer, got %v", err)
	}
	if err := IndexGPUPoolByName(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for nil indexer, got %v", err)
	}

	expected := errors.New("index failed")
	idx := &capturingFieldIndexer{err: expected}
	if err := IndexGPUDeviceByNode(context.Background(), idx); !errors.Is(err, expected) {
		t.Fatalf("expected error propagated, got %v", err)
	}
}

func TestIndexGPUDeviceByPoolRefNameSkipsEmptyPoolName(t *testing.T) {
	idx := &capturingFieldIndexer{}
	if err := IndexGPUDeviceByPoolRefName(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	device := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: ""}}}
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty poolRef name, got %+v", got)
	}
}

func TestIndexGPUDeviceAssignmentIndexersHandleEmptyValueAndOtherObjects(t *testing.T) {
	idx := &capturingFieldIndexer{}
	if err := IndexGPUDeviceByNamespacedAssignment(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"gpu.deckhouse.io/assignment": ""}}}
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty assignment, got %+v", got)
	}
	if got := idx.extractor(&corev1.Pod{}); got != nil {
		t.Fatalf("expected nil for other objects, got %+v", got)
	}

	idx = &capturingFieldIndexer{}
	if err := IndexGPUDeviceByClusterAssignment(context.Background(), idx); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	device = &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"cluster.gpu.deckhouse.io/assignment": ""}}}
	if got := idx.extractor(device); got != nil {
		t.Fatalf("expected nil for empty assignment, got %+v", got)
	}
	if got := idx.extractor(&corev1.Pod{}); got != nil {
		t.Fatalf("expected nil for other objects, got %+v", got)
	}
}
