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

package poolusage

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	testutil "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/internal/testutil"
)

type stubController struct {
	watchErrs []error
	calls     int
}

func (s *stubController) Reconcile(context.Context, ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (s *stubController) Watch(source.Source) error {
	s.calls++
	if s.calls-1 < len(s.watchErrs) && s.watchErrs[s.calls-1] != nil {
		return s.watchErrs[s.calls-1]
	}
	return nil
}

func (s *stubController) Start(context.Context) error { return nil }

func (s *stubController) GetLogger() logr.Logger { return logr.Discard() }

func TestGPUPoolUsageSetupControllerBranches(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("cache-required", func(t *testing.T) {
		r := NewGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: nil, Client: cl}

		if err := r.SetupController(ctx, mgr, &stubController{}); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("first-watch-error", func(t *testing.T) {
		r := NewGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: &testutil.FakeCache{}, Client: cl}

		watchErr := errors.New("watch fail")
		err := r.SetupController(ctx, mgr, &stubController{watchErrs: []error{watchErr}})
		if !errors.Is(err, watchErr) {
			t.Fatalf("expected watch error, got %v", err)
		}
	})

	t.Run("second-watch-error", func(t *testing.T) {
		r := NewGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: &testutil.FakeCache{}, Client: cl}

		watchErr := errors.New("watch fail")
		err := r.SetupController(ctx, mgr, &stubController{watchErrs: []error{nil, watchErr}})
		if !errors.Is(err, watchErr) {
			t.Fatalf("expected watch error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		r := NewGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: &testutil.FakeCache{}, Client: cl}

		if err := r.SetupController(ctx, mgr, &stubController{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.client != cl {
			t.Fatalf("expected reconciler client to be assigned from manager")
		}
	})
}

func TestClusterGPUPoolUsageSetupControllerBranches(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("cache-required", func(t *testing.T) {
		r := NewClusterGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: nil, Client: cl}

		if err := r.SetupController(ctx, mgr, &stubController{}); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("first-watch-error", func(t *testing.T) {
		r := NewClusterGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: &testutil.FakeCache{}, Client: cl}

		watchErr := errors.New("watch fail")
		err := r.SetupController(ctx, mgr, &stubController{watchErrs: []error{watchErr}})
		if !errors.Is(err, watchErr) {
			t.Fatalf("expected watch error, got %v", err)
		}
	})

	t.Run("second-watch-error", func(t *testing.T) {
		r := NewClusterGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: &testutil.FakeCache{}, Client: cl}

		watchErr := errors.New("watch fail")
		err := r.SetupController(ctx, mgr, &stubController{watchErrs: []error{nil, watchErr}})
		if !errors.Is(err, watchErr) {
			t.Fatalf("expected watch error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		r := NewClusterGPUPoolUsage(logr.Discard(), config.ControllerConfig{Workers: 0}, nil)
		mgr := &testutil.StubManager{Cache: &testutil.FakeCache{}, Client: cl}

		if err := r.SetupController(ctx, mgr, &stubController{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.client != cl {
			t.Fatalf("expected reconciler client to be assigned from manager")
		}
	})
}
