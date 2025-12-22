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

package nodemark

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
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

type nodeMarkListInterceptClient struct {
	client.Client
	fail func(list client.ObjectList, opts []client.ListOption) error
}

func (c nodeMarkListInterceptClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.fail != nil {
		if err := c.fail(list, opts); err != nil {
			return err
		}
	}
	return c.Client.List(ctx, list, opts...)
}

type nodeMarkPatchErrorClient struct {
	client.Client
	err error
}

func (c nodeMarkPatchErrorClient) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	return c.err
}

type nodeMarkListErrorClient struct {
	client.Client
	err error
}

func (c nodeMarkListErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

type nodeMarkGetErrorClient struct {
	client.Client
	err error
}

func (c nodeMarkGetErrorClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}

func TestNodeMarkHandlePoolSkipsIgnoredMismatchedAndNoNodeDevices(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}
	poolKey := poolcommon.PoolLabelKey(pool)

	okDev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "ok"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a"},
		},
	}
	ignoredDev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ignored",
			Labels: map[string]string{"gpu.deckhouse.io/ignore": "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a"},
		},
	}
	mismatchedRef := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "mismatch-ref"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns"},
		},
	}
	noNode := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "no-node"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool-a"},
		},
	}

	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
			okDev,
			ignoredDev,
			mismatchedRef,
			noNode,
		).
		Build()

	h := NewNodeMarkHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	node := &corev1.Node{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, node); err != nil {
		t.Fatalf("get: %v", err)
	}
	if node.Labels[poolKey] != "pool-a" {
		t.Fatalf("expected node to be marked with %q=pool-a, got %+v", poolKey, node.Labels)
	}
}

func TestNodeMarkHandlePoolReturnsDeviceListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewNodeMarkHandler(testr.New(t), nodeMarkListErrorClient{Client: base, err: errors.New("list error")})
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}); err == nil {
		t.Fatalf("expected devices list error")
	}
}

func TestNodeMarkHandlePoolReturnsTaintedNodesListErrorWithoutIndex(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithObjects(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}).
		Build()

	h := NewNodeMarkHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}); err == nil {
		t.Fatalf("expected tainted nodes list error")
	}
}

func TestNodeMarkHandlePoolReturnsNodeLabelListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}
	poolKey := poolcommon.PoolLabelKey(pool)
	poolValue := poolcommon.PoolValueFromKey(poolKey)

	base := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		Build()

	h := NewNodeMarkHandler(testr.New(t), nodeMarkListInterceptClient{
		Client: base,
		fail: func(list client.ObjectList, opts []client.ListOption) error {
			if _, ok := list.(*corev1.NodeList); !ok {
				return nil
			}
			for _, opt := range opts {
				labels, ok := opt.(client.MatchingLabels)
				if ok && labels[poolKey] == poolValue {
					return errors.New("boom")
				}
			}
			return nil
		},
	})

	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected node label list error")
	}
}

func TestNodeMarkHandlePoolPropagatesSyncNodeError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}

	base := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme))).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
			&v1alpha1.GPUDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "dev1"},
				Status: v1alpha1.GPUDeviceStatus{
					NodeName: "node1",
					PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool-a"},
				},
			},
		).
		Build()

	h := NewNodeMarkHandler(testr.New(t), nodeMarkPatchErrorClient{Client: base, err: errors.New("patch error")})
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected syncNode patch error")
	}
}

func TestNodeMarkSyncNodeReturnsGetError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewNodeMarkHandler(testr.New(t), nodeMarkGetErrorClient{Client: base, err: errors.New("boom")})

	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool-a", true, false); err == nil {
		t.Fatalf("expected get error")
	}
}

func TestNodeMarkSyncNodeNoopWhenNodeMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)

	if err := h.syncNode(context.Background(), "missing", "gpu.deckhouse.io/pool-a", true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
