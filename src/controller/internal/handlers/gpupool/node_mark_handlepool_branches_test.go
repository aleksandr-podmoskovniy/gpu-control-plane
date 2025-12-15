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

package gpupool

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

type nodeListCallErrorClient struct {
	client.Client
	err          error
	nodeListCall int
}

func (c *nodeListCallErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.NodeList); ok {
		c.nodeListCall++
		if c.nodeListCall == 3 {
			return c.err
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func TestHandlePoolDeviceLoopBranchesAndAltListItems(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
	}
	poolKey := poolLabelKey(pool)
	altPoolKey := alternatePoolLabelKey(pool)

	node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2", Labels: map[string]string{altPoolKey: "pool"}}}
	node3 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node3", Labels: map[string]string{poolKey: "pool"}}}

	devIgnored := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-ignored", Labels: map[string]string{deviceIgnoreKey: "true"}},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-ignored",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	devMismatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-mismatch"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node-mismatch",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"},
		},
	}
	devHostnameFallback := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-hostname", Labels: map[string]string{"kubernetes.io/hostname": "node1"}},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	devNoNodeInfo := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-nonode"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme))).
		WithObjects(node1, node2, node3, devIgnored, devMismatch, devHostnameFallback, devNoNodeInfo).
		Build()

	h := NewNodeMarkHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	loaded1 := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, loaded1)
	if loaded1.Labels[poolKey] != "pool" {
		t.Fatalf("expected node1 to get pool label via hostname fallback, got %+v", loaded1.Labels)
	}

	loaded2 := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node2"}, loaded2)
	if _, ok := loaded2.Labels[altPoolKey]; ok {
		t.Fatalf("expected node2 alt label removed, got %+v", loaded2.Labels)
	}

	loaded3 := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node3"}, loaded3)
	if _, ok := loaded3.Labels[poolKey]; ok {
		t.Fatalf("expected node3 pool label removed, got %+v", loaded3.Labels)
	}
}

func TestHandlePoolNodeListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	base := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme))).WithObjects(device).Build()
	badRequest := apierrors.NewBadRequest("node list failed")
	cl := &listErrorClient{Client: base, errs: map[string]error{"*v1.NodeList": badRequest}}

	h := NewNodeMarkHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected node list error, got %v", err)
	}
}

func TestHandlePoolAltListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	base := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme))).WithObjects(device).Build()

	cl := &nodeListCallErrorClient{Client: base, err: apierrors.NewBadRequest("alt list failed")}
	h := NewNodeMarkHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected alt list error, got %v", err)
	}
}
