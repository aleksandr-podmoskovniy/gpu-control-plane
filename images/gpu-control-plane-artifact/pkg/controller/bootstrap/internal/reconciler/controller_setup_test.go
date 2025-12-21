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

package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	bshandler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/handler"
)

func TestNewNormalisesWorkers(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{Workers: 0}, nil, nil)
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers defaulted to 1, got %d", rec.cfg.Workers)
	}
}

func TestSetupControllerRequiresCache(t *testing.T) {
	scheme := newScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(client, scheme)

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	if err := rec.SetupController(context.Background(), mgr, &fakeController{}); err == nil {
		t.Fatalf("expected error when manager cache is nil")
	}
}

func TestSetupControllerIndexesPodsWhenIndexerProvided(t *testing.T) {
	scheme := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}
	mgr.indexer = &stubFieldIndexer{}

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	ctr := &fakeController{log: logr.Discard()}

	if err := rec.SetupController(context.Background(), mgr, ctr); err != nil {
		t.Fatalf("SetupController returned error: %v", err)
	}
	idx := mgr.indexer.(*stubFieldIndexer)
	if len(idx.results) != 3 {
		t.Fatalf("expected 3 extractValue calls, got %d", len(idx.results))
	}
	if len(idx.results[0]) != 1 || idx.results[0][0] != "node-a" {
		t.Fatalf("unexpected extractValue result: %#v", idx.results[0])
	}
	if idx.results[1] != nil || idx.results[2] != nil {
		t.Fatalf("expected nil extractValue results for non-matching pod objects, got %#v", idx.results[1:3])
	}
	if len(ctr.watched) != 4 {
		t.Fatalf("expected 4 watch registrations (inventory + 3 watchers), got %d", len(ctr.watched))
	}
}

func TestSetupControllerPropagatesIndexerError(t *testing.T) {
	scheme := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}
	mgr.indexer = &stubFieldIndexer{err: errors.New("index fail")}

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	if err := rec.SetupController(context.Background(), mgr, &fakeController{}); err == nil {
		t.Fatalf("expected indexer error")
	}
}

func TestSetupControllerWithoutIndexer(t *testing.T) {
	scheme := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}
	mgr.indexer = nil

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	ctr := &fakeController{log: logr.Discard()}

	if err := rec.SetupController(context.Background(), mgr, ctr); err != nil {
		t.Fatalf("SetupController returned error: %v", err)
	}
	if len(ctr.watched) != 4 {
		t.Fatalf("expected 4 watch registrations (inventory + 3 watchers), got %d", len(ctr.watched))
	}
}

type failingWatchController struct {
	fakeController
	err error
}

func (f *failingWatchController) Watch(source.Source) error {
	return f.err
}

func TestSetupControllerPropagatesWatchError(t *testing.T) {
	scheme := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	if err := rec.SetupController(context.Background(), mgr, &failingWatchController{err: errors.New("watch fail")}); err == nil {
		t.Fatalf("expected watch error")
	}
}

type failOnSecondWatchController struct {
	fakeController
	calls int
	err   error
}

func (f *failOnSecondWatchController) Watch(src source.Source) error {
	f.calls++
	if f.calls == 2 {
		return f.err
	}
	return f.fakeController.Watch(src)
}

func TestSetupControllerPropagatesWatcherError(t *testing.T) {
	scheme := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	ctr := &failOnSecondWatchController{err: errors.New("watcher watch fail")}
	if err := rec.SetupController(context.Background(), mgr, ctr); err == nil {
		t.Fatalf("expected watcher error")
	}
}

func TestInjectClientAssignsOnlyHandlersWithSetter(t *testing.T) {
	scheme := newScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	withSetter := &clientAwareBootstrapHandler{}
	withoutSetter := &stubBootstrapHandler{name: "plain"}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, []Handler{bshandler.WrapBootstrapHandler(withSetter), bshandler.WrapBootstrapHandler(withoutSetter)})
	rec.client = cl

	rec.injectClient()

	if withSetter.client != cl {
		t.Fatal("expected handler with SetClient to receive client")
	}
}

func TestInjectClientNoClient(t *testing.T) {
	withSetter := &clientAwareBootstrapHandler{}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, []Handler{bshandler.WrapBootstrapHandler(withSetter)})
	rec.client = nil
	rec.injectClient()
	if withSetter.client != nil {
		t.Fatal("expected handler to remain nil when controller client is nil")
	}
}
