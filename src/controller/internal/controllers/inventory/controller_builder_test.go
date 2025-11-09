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

package inventory

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

type stubRuntimeAdapter struct {
	calls []string
	err   error
}

func (s *stubRuntimeAdapter) Named(name string) controllerRuntimeAdapter {
	s.calls = append(s.calls, "Named:"+name)
	return s
}

func (s *stubRuntimeAdapter) For(obj client.Object, _ ...builder.ForOption) controllerRuntimeAdapter {
	s.calls = append(s.calls, "For:"+fmt.Sprintf("%T", obj))
	return s
}

func (s *stubRuntimeAdapter) Owns(obj client.Object, _ ...builder.OwnsOption) controllerRuntimeAdapter {
	s.calls = append(s.calls, "Owns:"+fmt.Sprintf("%T", obj))
	return s
}

func (s *stubRuntimeAdapter) WatchesRawSource(source.Source) controllerRuntimeAdapter {
	s.calls = append(s.calls, "Watches")
	return s
}

func (s *stubRuntimeAdapter) WithOptions(opts controller.Options) controllerRuntimeAdapter {
	s.calls = append(s.calls, fmt.Sprintf("WithOptions:%d", opts.MaxConcurrentReconciles))
	return s
}

func (s *stubRuntimeAdapter) Complete(reconcile.Reconciler) error {
	s.calls = append(s.calls, "Complete")
	return s.err
}

func TestRuntimeControllerBuilderDelegates(t *testing.T) {
	adapter := &stubRuntimeAdapter{}
	builder := &runtimeControllerBuilder{adapter: adapter}

	if builder.Named("gpu") != builder {
		t.Fatalf("Named should return the same builder")
	}
	if builder.For(&v1alpha1.GPUDevice{}) != builder {
		t.Fatalf("For should return the same builder")
	}
	if builder.Owns(&v1alpha1.GPUNodeInventory{}) != builder {
		t.Fatalf("Owns should return the same builder")
	}
	if builder.WatchesRawSource(nil) != builder {
		t.Fatalf("WatchesRawSource should return the same builder")
	}
	if builder.WithOptions(controller.Options{MaxConcurrentReconciles: 2}) != builder {
		t.Fatalf("WithOptions should return the same builder")
	}

	err := builder.Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
		return reconcile.Result{}, nil
	}))
	if err != nil {
		t.Fatalf("Complete returned unexpected error: %v", err)
	}

	want := []string{
		"Named:gpu",
		"For:*v1alpha1.GPUDevice",
		"Owns:*v1alpha1.GPUNodeInventory",
		"Watches",
		"WithOptions:2",
		"Complete",
	}
	if diff := cmp.Diff(want, adapter.calls); diff != "" {
		t.Fatalf("unexpected call sequence (-want +got):\n%s", diff)
	}
}

func TestDefaultControllerBuilderUsesFactory(t *testing.T) {
	orig := newControllerManagedBy
	defer func() { newControllerManagedBy = orig }()

	stub := &stubRuntimeAdapter{}
	var called bool
	newControllerManagedBy = func(ctrl.Manager) controllerRuntimeAdapter {
		called = true
		return stub
	}

	builder := defaultControllerBuilder(nil)
	if !called {
		t.Fatalf("expected newControllerManagedBy to be invoked")
	}

	if builder.Named("inventory") != builder {
		t.Fatalf("Named should return builder")
	}
	if len(stub.calls) == 0 || stub.calls[0] != "Named:inventory" {
		t.Fatalf("expected Named to delegate to adapter, got %v", stub.calls)
	}
}

func TestControllerRuntimeWrapperWrapsBuilder(t *testing.T) {
	adapter := controllerRuntimeAdapter(&controllerRuntimeWrapper{builder: &builder.Builder{}})

	if next := adapter.Named("wrapper"); next != adapter {
		t.Fatalf("Named must return the same adapter instance")
	}
	adapter = adapter.For(&v1alpha1.GPUDevice{})
	adapter = adapter.Owns(&v1alpha1.GPUNodeInventory{})
	adapter = adapter.WatchesRawSource(nil)
	adapter = adapter.WithOptions(controller.Options{MaxConcurrentReconciles: 3})

	if err := adapter.Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
		return reconcile.Result{}, nil
	})); err == nil {
		t.Fatalf("expected error because underlying builder has no manager")
	}
}

func TestDefaultNodeFeatureSource(t *testing.T) {
	if src := defaultNodeFeatureSource(nil); src == nil {
		t.Fatalf("expected defaultNodeFeatureSource to return non-nil source")
	}
}

func TestNewControllerManagedByAcceptsNilManager(t *testing.T) {
	if adapter := newControllerManagedBy(nil); adapter == nil {
		t.Fatal("expected controller adapter even when manager is nil")
	}
}
