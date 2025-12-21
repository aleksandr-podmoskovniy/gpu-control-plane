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

package gpupool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	gpstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/gpupool/internal/state"
)

type fakeCache struct{ cache.Cache }

type fakeController struct {
	watched []source.Source
	log     logr.Logger
}

func (f *fakeController) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
func (f *fakeController) Watch(src source.Source) error {
	f.watched = append(f.watched, src)
	return nil
}
func (f *fakeController) Start(context.Context) error { return nil }
func (f *fakeController) GetLogger() logr.Logger      { return f.log }

type fakeManager struct {
	client  client.Client
	scheme  *runtime.Scheme
	cache   cache.Cache
	indexer client.FieldIndexer
	log     logr.Logger
}

func (f *fakeManager) GetClient() client.Client                        { return f.client }
func (f *fakeManager) GetScheme() *runtime.Scheme                      { return f.scheme }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer            { return f.indexer }
func (f *fakeManager) GetHTTPClient() *http.Client                     { return nil }
func (f *fakeManager) GetConfig() *rest.Config                         { return nil }
func (f *fakeManager) GetCache() cache.Cache                           { return f.cache }
func (f *fakeManager) GetEventRecorderFor(string) record.EventRecorder { return nil }
func (f *fakeManager) GetRESTMapper() meta.RESTMapper                  { return nil }
func (f *fakeManager) GetAPIReader() client.Reader                     { return nil }
func (f *fakeManager) Start(context.Context) error                     { return nil }
func (f *fakeManager) Add(manager.Runnable) error                      { return nil }
func (f *fakeManager) Elected() <-chan struct{}                        { return make(chan struct{}) }
func (f *fakeManager) AddMetricsServerExtraHandler(string, http.Handler) error {
	return nil
}
func (f *fakeManager) AddHealthzCheck(string, healthz.Checker) error { return nil }
func (f *fakeManager) AddReadyzCheck(string, healthz.Checker) error  { return nil }
func (f *fakeManager) GetWebhookServer() webhook.Server              { return nil }
func (f *fakeManager) GetLogger() logr.Logger                        { return f.log }
func (f *fakeManager) GetControllerOptions() ctrlconfig.Controller   { return ctrlconfig.Controller{} }

type stubFieldIndexer struct {
	calls  int
	err    error
	failAt int
}

func (s *stubFieldIndexer) IndexField(_ context.Context, _ client.Object, _ string, _ client.IndexerFunc) error {
	s.calls++
	if s.failAt > 0 && s.calls == s.failAt {
		return s.err
	}
	return nil
}

type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return f.err
}

type stubHandler struct {
	name   string
	result reconcile.Result
	err    error
	calls  int
}

func (s *stubHandler) Name() string { return s.name }
func (s *stubHandler) Handle(_ context.Context, st gpstate.PoolState) (reconcile.Result, error) {
	s.calls++
	if st != nil && st.Pool() != nil {
		st.Pool().Status.Capacity.Total++
	}
	return s.result, s.err
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestNewNormalisesWorkers(t *testing.T) {
	rec := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 0}, nil, nil)
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers defaulted to 1, got %d", rec.cfg.Workers)
	}
}

func TestSetupControllerRequiresCache(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)

	mgr := &fakeManager{client: cl, scheme: scheme, cache: nil}
	if err := rec.SetupController(context.Background(), mgr, &fakeController{}); err == nil {
		t.Fatalf("expected error when manager cache is nil")
	}
}

func TestSetupControllerIndexesFieldsAndRegistersWatches(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	idx := &stubFieldIndexer{}
	mgr := &fakeManager{client: cl, scheme: scheme, cache: &fakeCache{}, indexer: idx, log: logr.Discard()}
	ctr := &fakeController{log: logr.Discard()}

	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	if err := rec.SetupController(context.Background(), mgr, ctr); err != nil {
		t.Fatalf("SetupController returned error: %v", err)
	}
	if rec.client != cl {
		t.Fatalf("expected manager client to be captured")
	}
	if idx.calls != 5 {
		t.Fatalf("expected 5 index registrations, got %d", idx.calls)
	}
	if len(ctr.watched) != 4 {
		t.Fatalf("expected 4 watch registrations (pool + 3 watchers), got %d", len(ctr.watched))
	}
}

func TestSetupControllerWithoutIndexer(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := &fakeManager{client: cl, scheme: scheme, cache: &fakeCache{}, indexer: nil, log: logr.Discard()}
	ctr := &fakeController{log: logr.Discard()}

	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	if err := rec.SetupController(context.Background(), mgr, ctr); err != nil {
		t.Fatalf("SetupController returned error: %v", err)
	}
	if len(ctr.watched) != 4 {
		t.Fatalf("expected 4 watch registrations (pool + 3 watchers), got %d", len(ctr.watched))
	}
}

func TestSetupControllerPropagatesIndexerError(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	for failAt := 1; failAt <= 5; failAt++ {
		t.Run(fmt.Sprintf("failAt=%d", failAt), func(t *testing.T) {
			idx := &stubFieldIndexer{failAt: failAt, err: errors.New("index fail")}
			mgr := &fakeManager{client: cl, scheme: scheme, cache: &fakeCache{}, indexer: idx, log: logr.Discard()}

			rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
			if err := rec.SetupController(context.Background(), mgr, &fakeController{log: logr.Discard()}); err == nil {
				t.Fatalf("expected indexer error")
			}
		})
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
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := &fakeManager{client: cl, scheme: scheme, cache: &fakeCache{}, log: logr.Discard()}
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)

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
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := &fakeManager{client: cl, scheme: scheme, cache: &fakeCache{}, log: logr.Discard()}
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)

	if err := rec.SetupController(context.Background(), mgr, &failOnSecondWatchController{err: errors.New("watcher fail")}); err == nil {
		t.Fatalf("expected watcher error")
	}
}

func TestReconcileAggregatesResultsAndPersistsStatus(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(pool).
		Build()

	handlerA := &stubHandler{name: "a", result: reconcile.Result{Requeue: true}}
	handlerB := &stubHandler{name: "b", result: reconcile.Result{RequeueAfter: time.Second}}

	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, []Handler{handlerA, handlerB})
	rec.client = cl

	res, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != time.Second {
		t.Fatalf("unexpected aggregate result: %+v", res)
	}
	if handlerA.calls != 1 || handlerB.calls != 1 {
		t.Fatalf("expected handlers invoked once, got %d/%d", handlerA.calls, handlerB.calls)
	}

	updated := &v1alpha1.GPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "pool"}, updated); err != nil {
		t.Fatalf("get updated pool: %v", err)
	}
	if updated.Status.Capacity.Total != 2 {
		t.Fatalf("expected status updates to be persisted, got %d", updated.Status.Capacity.Total)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).WithStatusSubresource(pool).Build()

	handler := &stubHandler{name: "boom", err: errors.New("handler fail")}
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, []Handler{handler})
	rec.client = cl

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatalf("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("expected handler called once, got %d", handler.calls)
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = cl

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "missing"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReconcileGetError(t *testing.T) {
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: errors.New("get fail")}

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatalf("expected get error")
	}
}

func TestReconcileWrapsAPIError(t *testing.T) {
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpupools"}, "pool", errors.New("boom"))}

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatalf("expected API error")
	}
}
