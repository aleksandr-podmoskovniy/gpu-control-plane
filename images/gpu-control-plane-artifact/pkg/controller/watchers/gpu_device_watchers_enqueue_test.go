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

package watchers

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
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

func TestGPUPoolGPUDeviceWatcherEnqueueBranches(t *testing.T) {
	ctx := context.Background()
	w := NewGPUPoolGPUDeviceWatcher(testr.New(t))

	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests, got %#v", got)
	}

	dev := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"}},
	}
	reqs := w.enqueue(ctx, dev)
	if len(reqs) != 1 || reqs[0].Namespace != "ns" || reqs[0].Name != "pool" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}

	dev = &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"}}}
	if got := w.enqueue(ctx, dev); got != nil {
		t.Fatalf("expected nil requests when client is nil and pool is unqualified, got %#v", got)
	}

	w.enqueuer = NewGPUPoolGPUDeviceEnqueuer(w.log, &failingListClient{err: errors.New("list fail")})
	dev = &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev", Annotations: map[string]string{commonannotations.GPUDeviceAssignment: "pool"}}}
	if got := w.enqueue(ctx, dev); got != nil {
		t.Fatalf("expected nil requests on list error, got %#v", got)
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&v1alpha1.GPUPool{}, indexer.GPUPoolNameField, func(obj client.Object) []string {
			pool, ok := obj.(*v1alpha1.GPUPool)
			if !ok || pool.Name == "" {
				return nil
			}
			return []string{pool.Name}
		}).
		WithObjects(
			&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns1"}},
			&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns2"}},
		).
		Build()

	w.enqueuer = NewGPUPoolGPUDeviceEnqueuer(w.log, cl)
	dev = &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"}}}
	if got := w.enqueue(ctx, dev); got != nil {
		t.Fatalf("expected nil requests for unqualified poolRef, got %#v", got)
	}

	dev = &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev", Annotations: map[string]string{commonannotations.GPUDeviceAssignment: "pool"}}}
	reqs = w.enqueue(ctx, dev)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %#v", reqs)
	}
	want := map[string]struct{}{"ns1/pool": {}, "ns2/pool": {}}
	for _, req := range reqs {
		key := req.Namespace + "/" + req.Name
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected request: %#v", req)
		}
	}

	w.enqueuer = NewGPUPoolGPUDeviceEnqueuer(w.log, fake.NewClientBuilder().WithScheme(scheme).Build())
	if got := w.enqueue(ctx, dev); got != nil {
		t.Fatalf("expected nil requests when no pools are found, got %#v", got)
	}
}

func TestClusterGPUPoolGPUDeviceWatcherEnqueueBranches(t *testing.T) {
	ctx := context.Background()
	w := NewClusterGPUPoolGPUDeviceWatcher(testr.New(t))

	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests, got %#v", got)
	}

	dev := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"}}}
	reqs := w.enqueue(ctx, dev)
	if len(reqs) != 1 || reqs[0].Name != "pool" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}

	dev.Status.PoolRef.Namespace = "ns"
	if got := w.enqueue(ctx, dev); got != nil {
		t.Fatalf("expected namespaced poolRef to be ignored, got %#v", got)
	}

	dev = &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{commonannotations.ClusterGPUDeviceAssignment: "pool"}}}
	reqs = w.enqueue(ctx, dev)
	if len(reqs) != 1 || reqs[0].Name != "pool" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}

	if got := w.enqueue(ctx, &v1alpha1.GPUDevice{}); got != nil {
		t.Fatalf("expected nil requests when no assignment, got %#v", got)
	}
}
