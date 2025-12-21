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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type failingListClient struct {
	client.Client
	err error
}

func (c *failingListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

func TestModuleConfigResyncWatchersMapFuncs(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	nsPool := &v1alpha1.GPUPool{}
	nsPool.Name = "pool"
	nsPool.Namespace = "ns"

	clusterPool := &v1alpha1.ClusterGPUPool{}
	clusterPool.Name = "pool"

	nodeState := &v1alpha1.GPUNodeState{}
	nodeState.Name = "node-a"

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nsPool, clusterPool, nodeState).Build()

	t.Run("gpupool", func(t *testing.T) {
		w := NewGPUPoolModuleConfigWatcher(logr.Discard(), nil)
		reqs, err := w.mapFunc(ctx, cl, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reqs) != 1 || reqs[0].Namespace != "ns" || reqs[0].Name != "pool" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
	})

	t.Run("clustergpupool", func(t *testing.T) {
		w := NewClusterGPUPoolModuleConfigWatcher(logr.Discard(), nil)
		reqs, err := w.mapFunc(ctx, cl, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reqs) != 1 || reqs[0].Name != "pool" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
	})

	t.Run("gpunodestate", func(t *testing.T) {
		w := NewGPUNodeStateModuleConfigWatcher(logr.Discard(), nil)
		reqs, err := w.mapFunc(ctx, cl, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reqs) != 1 || reqs[0].Name != "node-a" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
	})
}

func TestModuleConfigResyncWatchersMapFuncErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("list fail")
	cl := &failingListClient{err: boom}

	w := NewGPUPoolModuleConfigWatcher(logr.Discard(), nil)
	if _, err := w.mapFunc(ctx, cl, nil); !errors.Is(err, boom) {
		t.Fatalf("expected list error, got %v", err)
	}

	w = NewClusterGPUPoolModuleConfigWatcher(logr.Discard(), nil)
	if _, err := w.mapFunc(ctx, cl, nil); !errors.Is(err, boom) {
		t.Fatalf("expected list error, got %v", err)
	}

	w = NewGPUNodeStateModuleConfigWatcher(logr.Discard(), nil)
	if _, err := w.mapFunc(ctx, cl, nil); !errors.Is(err, boom) {
		t.Fatalf("expected list error, got %v", err)
	}
}
