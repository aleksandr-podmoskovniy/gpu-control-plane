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

package moduleconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type fakeCache struct{ cache.Cache }

type recordingController struct {
	watched []source.Source
	err     error
}

func (c *recordingController) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (c *recordingController) Watch(src source.Source) error {
	if c.err != nil {
		return c.err
	}
	c.watched = append(c.watched, src)
	return nil
}

func (c *recordingController) Start(context.Context) error { return nil }

func (c *recordingController) GetLogger() logr.Logger { return logr.Discard() }

var _ controller.Controller = (*recordingController)(nil)

func TestReconcilerSetupControllerRegistersWatch(t *testing.T) {
	scheme := newModuleConfigScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}

	rec, err := New(cl, testr.New(t), NewModuleConfigStore(DefaultState()))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctr := &recordingController{}
	if err := rec.SetupController(context.Background(), mgr, ctr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctr.watched) != 1 {
		t.Fatalf("expected 1 watch registration, got %d", len(ctr.watched))
	}
}

func TestSetupControllerSetupBranches(t *testing.T) {
	ctx := context.Background()
	scheme := newModuleConfigScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("requires-store", func(t *testing.T) {
		mgr := newFakeManager(cl, scheme)
		if err := SetupController(ctx, mgr, testr.New(t), nil); err == nil {
			t.Fatalf("expected store required error")
		}
	})

	t.Run("propagates-controller-add-error", func(t *testing.T) {
		mgr := newFakeManager(cl, scheme)
		mgr.addErr = errors.New("add fail")
		if err := SetupController(ctx, mgr, testr.New(t), NewModuleConfigStore(DefaultState())); !errors.Is(err, mgr.addErr) {
			t.Fatalf("expected add error, got %v", err)
		}
	})

	t.Run("propagates-reconciler-setup-error", func(t *testing.T) {
		mgr := newFakeManager(cl, scheme)
		mgr.cache = nil
		if err := SetupController(ctx, mgr, testr.New(t), NewModuleConfigStore(DefaultState())); err == nil {
			t.Fatalf("expected setup error")
		}
	})

	t.Run("success", func(t *testing.T) {
		mgr := newFakeManager(cl, scheme)
		mgr.cache = &fakeCache{}
		if err := SetupController(ctx, mgr, testr.New(t), NewModuleConfigStore(DefaultState())); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
