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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
)

type capturingFieldIndexer struct {
	fields []string
	err    error
}

func (f *capturingFieldIndexer) IndexField(_ context.Context, _ client.Object, field string, _ client.IndexerFunc) error {
	f.fields = append(f.fields, field)
	return f.err
}

type stubManager struct {
	manager.Manager
	fieldIndexer client.FieldIndexer
}

func (s *stubManager) GetFieldIndexer() client.FieldIndexer {
	return s.fieldIndexer
}

func TestIndexGPUDeviceByNode(t *testing.T) {
	obj, field, extractor := IndexGPUDeviceByNode()
	if _, ok := obj.(*v1alpha1.GPUDevice); !ok {
		t.Fatalf("expected GPUDevice object, got %T", obj)
	}
	if field != GPUDeviceNodeField {
		t.Fatalf("expected field %s, got %s", GPUDeviceNodeField, field)
	}
	device := &v1alpha1.GPUDevice{}
	device.Status.NodeName = "node-a"
	values := extractor(device)
	if !reflect.DeepEqual(values, []string{"node-a"}) {
		t.Fatalf("expected node name indexed, got %+v", values)
	}

	// empty node name -> no index
	device.Status.NodeName = ""
	if got := extractor(device); got != nil {
		t.Fatalf("expected nil for empty node name, got %+v", got)
	}

	// unrelated object should be ignored
	pod := &corev1.Pod{}
	if got := extractor(pod); got != nil {
		t.Fatalf("expected nil for non-GPUDevice object, got %+v", got)
	}
}

func TestIndexGPUDeviceByPoolRefName(t *testing.T) {
	obj, field, extractor := IndexGPUDeviceByPoolRefName()
	if _, ok := obj.(*v1alpha1.GPUDevice); !ok {
		t.Fatalf("expected GPUDevice object, got %T", obj)
	}
	if field != GPUDevicePoolRefNameField {
		t.Fatalf("expected field %s, got %s", GPUDevicePoolRefNameField, field)
	}

	device := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool-a"}}}
	if got := extractor(device); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected pool name indexed, got %+v", got)
	}

	device.Status.PoolRef = nil
	if got := extractor(device); got != nil {
		t.Fatalf("expected nil for empty pool ref, got %+v", got)
	}

	// unrelated object should be ignored
	pod := &corev1.Pod{}
	if got := extractor(pod); got != nil {
		t.Fatalf("expected nil for non-GPUDevice object, got %+v", got)
	}
}

func TestIndexGPUDeviceByNamespacedAssignment(t *testing.T) {
	obj, field, extractor := IndexGPUDeviceByNamespacedAssignment()
	if _, ok := obj.(*v1alpha1.GPUDevice); !ok {
		t.Fatalf("expected GPUDevice object, got %T", obj)
	}
	if field != GPUDeviceNamespacedAssignmentField {
		t.Fatalf("expected field %s, got %s", GPUDeviceNamespacedAssignmentField, field)
	}

	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{commonannotations.GPUDeviceAssignment: "pool-a"}}}
	if got := extractor(device); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected assignment indexed, got %+v", got)
	}

	device.Annotations = nil
	if got := extractor(device); got != nil {
		t.Fatalf("expected nil for empty annotations, got %+v", got)
	}
}

func TestIndexGPUDeviceByClusterAssignment(t *testing.T) {
	obj, field, extractor := IndexGPUDeviceByClusterAssignment()
	if _, ok := obj.(*v1alpha1.GPUDevice); !ok {
		t.Fatalf("expected GPUDevice object, got %T", obj)
	}
	if field != GPUDeviceClusterAssignmentField {
		t.Fatalf("expected field %s, got %s", GPUDeviceClusterAssignmentField, field)
	}

	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{commonannotations.ClusterGPUDeviceAssignment: "pool-a"}}}
	if got := extractor(device); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected assignment indexed, got %+v", got)
	}

	device.Annotations = nil
	if got := extractor(device); got != nil {
		t.Fatalf("expected nil for empty annotations, got %+v", got)
	}
}

func TestIndexGPUPoolByName(t *testing.T) {
	obj, field, extractor := IndexGPUPoolByName()
	if _, ok := obj.(*v1alpha1.GPUPool); !ok {
		t.Fatalf("expected GPUPool object, got %T", obj)
	}
	if field != GPUPoolNameField {
		t.Fatalf("expected field %s, got %s", GPUPoolNameField, field)
	}

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	if got := extractor(pool); len(got) != 1 || got[0] != "pool-a" {
		t.Fatalf("expected pool name indexed, got %+v", got)
	}

	pool.Name = ""
	if got := extractor(pool); got != nil {
		t.Fatalf("expected nil for empty pool name, got %+v", got)
	}

	if got := extractor(&corev1.Pod{}); got != nil {
		t.Fatalf("expected nil for non-GPUPool object, got %+v", got)
	}
}

func TestIndexNodeByTaintKey(t *testing.T) {
	obj, field, extractor := IndexNodeByTaintKey()
	if _, ok := obj.(*corev1.Node); !ok {
		t.Fatalf("expected Node object, got %T", obj)
	}
	if field != NodeTaintKeyField {
		t.Fatalf("expected field %s, got %s", NodeTaintKeyField, field)
	}

	t.Run("no-taints", func(t *testing.T) {
		node := &corev1.Node{}
		if got := extractor(node); got != nil {
			t.Fatalf("expected nil for node without taints, got %+v", got)
		}
	})

	t.Run("dedup-and-skip-empty", func(t *testing.T) {
		node := &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
			{Key: "a"},
			{Key: ""},
			{Key: "a"},
			{Key: "b"},
		}}}
		got := extractor(node)
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("unexpected taint keys: %+v", got)
		}
	})

	t.Run("all-empty-keys", func(t *testing.T) {
		node := &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{{Key: ""}}}}
		if got := extractor(node); got != nil {
			t.Fatalf("expected nil for node taints without keys, got %+v", got)
		}
	})

	t.Run("non-node-object", func(t *testing.T) {
		if got := extractor(&corev1.Pod{}); got != nil {
			t.Fatalf("expected nil for non-node object, got %+v", got)
		}
	})
}

func TestIndexALLRegistersIndexers(t *testing.T) {
	idx := &capturingFieldIndexer{}
	mgr := &stubManager{fieldIndexer: idx}

	if err := IndexALL(context.Background(), mgr); err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}

	expected := make([]string, 0, len(IndexGetters))
	for _, getter := range IndexGetters {
		_, field, _ := getter()
		expected = append(expected, field)
	}

	if !reflect.DeepEqual(idx.fields, expected) {
		t.Fatalf("unexpected registered fields: %+v", idx.fields)
	}
}

func TestIndexALLPropagatesErrors(t *testing.T) {
	idx := &capturingFieldIndexer{err: errors.New("index failed")}
	mgr := &stubManager{fieldIndexer: idx}

	if err := IndexALL(context.Background(), mgr); err == nil {
		t.Fatalf("expected index error")
	}
}
