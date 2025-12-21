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
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type fakeCache struct{ cache.Cache }

type stubFieldIndexer struct {
	client.FieldIndexer
	err   error
	calls int
}

func (s *stubFieldIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractor client.IndexerFunc) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	return nil
}

type stubManager struct {
	manager.Manager
	cache        cache.Cache
	client       client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	fieldIndexer client.FieldIndexer
}

func (m *stubManager) GetCache() cache.Cache { return m.cache }

func (m *stubManager) GetClient() client.Client { return m.client }

func (m *stubManager) GetScheme() *runtime.Scheme { return m.scheme }

func (m *stubManager) GetEventRecorderFor(string) record.EventRecorder { return m.recorder }

func (m *stubManager) GetFieldIndexer() client.FieldIndexer { return m.fieldIndexer }

type stubController struct {
	watched []source.Source
	err     error
}

func (c *stubController) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (c *stubController) Watch(src source.Source) error {
	if c.err != nil {
		return c.err
	}
	c.watched = append(c.watched, src)
	return nil
}

func (c *stubController) Start(context.Context) error { return nil }

func (c *stubController) GetLogger() logr.Logger { return logr.Discard() }

var _ controller.Controller = (*stubController)(nil)

func TestSetupControllerRequiresCache(t *testing.T) {
	rec := &Reconciler{log: testr.New(t)}

	scheme := newTestScheme(t)
	mgr := &stubManager{
		cache:        nil,
		client:       newTestClient(scheme),
		scheme:       scheme,
		recorder:     record.NewFakeRecorder(1),
		fieldIndexer: &stubFieldIndexer{},
	}

	if err := rec.SetupController(context.Background(), mgr, &stubController{}); err == nil {
		t.Fatalf("expected cache required error")
	}
}

func TestSetupControllerPropagatesIndexerError(t *testing.T) {
	rec := &Reconciler{log: testr.New(t)}

	scheme := newTestScheme(t)
	idxErr := errors.New("index fail")
	mgr := &stubManager{
		cache:        &fakeCache{},
		client:       newTestClient(scheme),
		scheme:       scheme,
		recorder:     record.NewFakeRecorder(1),
		fieldIndexer: &stubFieldIndexer{err: idxErr},
	}

	if err := rec.SetupController(context.Background(), mgr, &stubController{}); !errors.Is(err, idxErr) {
		t.Fatalf("expected index error, got %v", err)
	}
}

func TestSetupControllerWrapsWatcherError(t *testing.T) {
	rec := &Reconciler{log: testr.New(t)}

	scheme := newTestScheme(t)
	mgr := &stubManager{
		cache:        &fakeCache{},
		client:       newTestClient(scheme),
		scheme:       scheme,
		recorder:     record.NewFakeRecorder(1),
		fieldIndexer: &stubFieldIndexer{},
	}

	watchErr := errors.New("watch fail")
	err := rec.SetupController(context.Background(), mgr, &stubController{err: watchErr})
	if err == nil {
		t.Fatalf("expected watcher error")
	}
	if !strings.Contains(err.Error(), "failed to run watcher NodeWatcher") {
		t.Fatalf("unexpected wrapped error: %v", err)
	}
}

func TestSetupControllerInitializesServices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"gpu.deckhouse.io/present": "true"}}}
	cl := newTestClient(scheme, node)

	idx := &stubFieldIndexer{}
	mgr := &stubManager{
		cache:        &fakeCache{},
		client:       cl,
		scheme:       scheme,
		recorder:     record.NewFakeRecorder(4),
		fieldIndexer: idx,
	}

	rec := &Reconciler{log: testr.New(t)}
	ctr := &stubController{}

	if err := rec.SetupController(context.Background(), mgr, ctr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.client != cl {
		t.Fatalf("expected client set from manager")
	}
	if rec.scheme != scheme {
		t.Fatalf("expected scheme set from manager")
	}
	if rec.recorder == nil {
		t.Fatalf("expected recorder set from manager")
	}
	if rec.detectionCollector == nil || rec.cleanupService == nil || rec.deviceService == nil || rec.inventoryService == nil {
		t.Fatalf("expected services initialized")
	}
	if idx.calls != 1 {
		t.Fatalf("expected indexer to be invoked once, got %d", idx.calls)
	}
	if len(ctr.watched) != 5 {
		t.Fatalf("expected 5 watchers to be registered, got %d", len(ctr.watched))
	}

	reqs, err := rec.mapModuleConfigRequests(context.Background(), cl, nil)
	if err != nil {
		t.Fatalf("unexpected map error: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Name != "node-a" {
		t.Fatalf("unexpected map requests: %#v", reqs)
	}
}
