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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

func TestGPUPoolValidatorPodWatcherEnqueueBranches(t *testing.T) {
	ctx := context.Background()
	w := NewGPUPoolValidatorPodWatcher(testr.New(t))

	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests, got %#v", got)
	}

	nonValidator := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "other", "pool": "pool"}}}
	if got := w.enqueue(ctx, nonValidator); got != nil {
		t.Fatalf("expected non-validator pod to be ignored, got %#v", got)
	}

	validatorNoPool := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nvidia-operator-validator"}}}
	if got := w.enqueue(ctx, validatorNoPool); got != nil {
		t.Fatalf("expected pod without pool label to be ignored, got %#v", got)
	}

	validator := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Labels: map[string]string{"app": "nvidia-operator-validator", "pool": "pool"}}}
	if got := w.enqueue(ctx, validator); got != nil {
		t.Fatalf("expected nil when client is nil, got %#v", got)
	}

	w.enqueuer = NewGPUPoolValidatorPodEnqueuer(w.log, &failingListClient{err: errors.New("list fail")})
	if got := w.enqueue(ctx, validator); got != nil {
		t.Fatalf("expected nil on list error, got %#v", got)
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
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

	w.enqueuer = NewGPUPoolValidatorPodEnqueuer(w.log, cl)
	reqs := w.enqueue(ctx, validator)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %#v", reqs)
	}
}

func TestClusterGPUPoolValidatorPodWatcherEnqueueBranches(t *testing.T) {
	ctx := context.Background()
	w := NewClusterGPUPoolValidatorPodWatcher(testr.New(t))

	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests, got %#v", got)
	}

	validator := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nvidia-operator-validator", "pool": "pool"}}}
	reqs := w.enqueue(ctx, validator)
	if len(reqs) != 1 || reqs[0].Name != "pool" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
}
