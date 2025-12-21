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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

type noopClient struct{ client.Client }

func TestModuleConfigWatcherWatchBranches(t *testing.T) {
	w := NewModuleConfigWatcher(testr.New(t), nil, "", func(context.Context, client.Client, *unstructured.Unstructured) ([]reconcile.Request, error) {
		return nil, nil
	})

	t.Run("requires-cache", func(t *testing.T) {
		if err := w.Watch(&stubManager{cache: nil}, &stubController{}); err == nil {
			t.Fatalf("expected cache required error")
		}
	})

	t.Run("requires-map-func", func(t *testing.T) {
		w2 := NewModuleConfigWatcher(testr.New(t), nil, "", nil)
		if err := w2.Watch(&stubManager{cache: &fakeCache{}}, &stubController{}); err == nil {
			t.Fatalf("expected map func required error")
		}
	})

	t.Run("propagates-watch-error", func(t *testing.T) {
		if err := w.Watch(&stubManager{cache: &fakeCache{}}, &stubController{err: errors.New("watch fail")}); err == nil {
			t.Fatalf("expected watch error")
		}
	})

	t.Run("registers-watch", func(t *testing.T) {
		ctr := &stubController{}
		if err := w.Watch(&stubManager{cache: &fakeCache{}}, ctr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ctr.watched) != 1 {
			t.Fatalf("expected 1 watch, got %d", len(ctr.watched))
		}
	})
}

func TestModuleConfigWatcherEnqueueBranches(t *testing.T) {
	ctx := context.Background()

	w := NewModuleConfigWatcher(testr.New(t), nil, "", func(context.Context, client.Client, *unstructured.Unstructured) ([]reconcile.Request, error) {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "ignored"}}}, nil
	})
	w.cl = nil
	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests when client is nil, got %#v", got)
	}

	state := moduleconfig.DefaultState()
	state.Enabled = false
	store := moduleconfig.NewModuleConfigStore(state)

	w = NewModuleConfigWatcher(testr.New(t), store, "", func(context.Context, client.Client, *unstructured.Unstructured) ([]reconcile.Request, error) {
		t.Fatalf("mapFunc should not be called when module disabled")
		return nil, nil
	})
	w.cl = nil
	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests, got %#v", got)
	}

	w = NewModuleConfigWatcher(testr.New(t), nil, "", func(context.Context, client.Client, *unstructured.Unstructured) ([]reconcile.Request, error) {
		return nil, errors.New("map fail")
	})
	w.cl = &noopClient{}
	if got := w.enqueue(ctx, nil); got != nil {
		t.Fatalf("expected nil requests on error, got %#v", got)
	}

	want := []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "x"}}}
	w = NewModuleConfigWatcher(testr.New(t), nil, "", func(context.Context, client.Client, *unstructured.Unstructured) ([]reconcile.Request, error) {
		return want, nil
	})
	w.cl = &noopClient{}
	got := w.enqueue(ctx, nil)
	if len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("unexpected requests: %#v", got)
	}
}
