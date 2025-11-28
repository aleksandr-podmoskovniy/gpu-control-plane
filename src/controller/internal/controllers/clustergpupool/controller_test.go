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

package clustergpupool

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type fakePoolHandler struct {
	calls int
	err   error
}

func (f *fakePoolHandler) Name() string { return "fake" }
func (f *fakePoolHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	f.calls++
	if f.err != nil {
		return contracts.Result{}, f.err
	}
	pool.Status.Capacity.Total = 1
	return contracts.Result{}, nil
}

type fakeBuilder struct {
	completeCalled bool
	watched        bool
}

func (f *fakeBuilder) Named(string) controllerBuilder { return f }
func (f *fakeBuilder) For(client.Object, ...builder.ForOption) controllerBuilder {
	return f
}
func (f *fakeBuilder) WithOptions(controller.Options) controllerBuilder { return f }
func (f *fakeBuilder) WatchesRawSource(source.Source) controllerBuilder {
	f.watched = true
	return f
}
func (f *fakeBuilder) Complete(reconcile.Reconciler) error {
	f.completeCalled = true
	return nil
}

type fakeCache struct{ cache.Cache }

// Fake manager implementing minimal methods used by SetupWithManager.
type fakeManager struct {
	client client.Client
	scheme *runtime.Scheme
	cache  cache.Cache
	log    logr.Logger
}

func (f *fakeManager) GetClient() client.Client                        { return f.client }
func (f *fakeManager) GetScheme() *runtime.Scheme                      { return f.scheme }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer            { return nil }
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

func TestSetupWithManager(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	fb := &fakeBuilder{}
	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerBuilder { return fb }
	rec.moduleWatcherFactory = func(c cache.Cache, b controllerBuilder) controllerBuilder {
		return b.WatchesRawSource(source.Kind(c, &unstructured.Unstructured{}, nil))
	}

	mgr := &fakeManager{client: cl, scheme: scheme, cache: &fakeCache{}, log: testr.New(t)}
	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.watched || !fb.completeCalled {
		t.Fatalf("builder hooks not invoked")
	}
}

func TestReconcileClusterPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.ClusterGPUPool{}).
		WithObjects(pool).
		Build()

	handler := &fakePoolHandler{}
	rec := &Reconciler{
		client:   cl,
		scheme:   scheme,
		log:      testr.New(t),
		cfg:      config.ControllerConfig{Workers: 1},
		handlers: []contracts.PoolHandler{handler},
	}

	if err := cl.Get(context.Background(), types.NamespacedName{Name: "pool"}, &v1alpha1.ClusterGPUPool{}); err != nil {
		t.Fatalf("fake client did not return seeded pool: %v", err)
	}

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("unexpected requeue %v", res)
	}
	if handler.calls != 1 {
		t.Fatalf("handler should be called once, got %d", handler.calls)
	}

	updated := &v1alpha1.ClusterGPUPool{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "pool"}, updated); err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if updated.Status.Capacity.Total != 1 {
		t.Fatalf("status not updated")
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	rec := &Reconciler{client: cl, scheme: scheme, log: testr.New(t), cfg: config.ControllerConfig{Workers: 1}}

	_, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "absent"}})
	if err != nil {
		t.Fatalf("not found should not error: %v", err)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	pool := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.ClusterGPUPool{}).WithObjects(pool).Build()
	handler := &fakePoolHandler{err: errors.New("boom")}
	rec := &Reconciler{client: cl, scheme: scheme, log: testr.New(t), handlers: []contracts.PoolHandler{handler}, cfg: config.ControllerConfig{Workers: 1}}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatalf("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("handler should be invoked once")
	}
}

func TestRequeueAllPoolsAndMapModuleConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	p1 := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a"}}
	p2 := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "b"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(p1, p2).Build()

	rec := &Reconciler{client: cl, log: testr.New(t)}
	reqs := rec.requeueAllPools(context.Background())
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	// mapModuleConfig should delegate to requeueAllPools
	got := rec.mapModuleConfig(context.Background(), &unstructured.Unstructured{})
	if len(got) != 2 {
		t.Fatalf("mapModuleConfig should requeue all pools")
	}
}

func TestAttachModuleWatcher(t *testing.T) {
	rec := &Reconciler{log: logr.Discard()}
	fb := &fakeBuilder{}
	fb2 := rec.attachModuleWatcher(fb, &fakeCache{})
	if fb2 != fb || !fb.watched {
		t.Fatalf("module watcher should configure builder")
	}
}

func TestNewSetsDefaultWorkers(t *testing.T) {
	rec := New(logr.Discard(), config.ControllerConfig{Workers: 0}, nil, nil)
	if rec.cfg.Workers != 1 {
		t.Fatalf("workers default should be 1")
	}
	if rec.moduleWatcherFactory == nil {
		t.Fatalf("moduleWatcherFactory must be set")
	}
	fb := &fakeBuilder{}
	rec.moduleWatcherFactory(&fakeCache{}, fb)
	if !fb.watched {
		t.Fatalf("moduleWatcherFactory should set watch")
	}
}

func TestNewControllerManagedBy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	mgr := &fakeManager{scheme: scheme, client: clientfake.NewClientBuilder().WithScheme(scheme).Build(), cache: &fakeCache{}, log: logr.Discard()}
	b := newControllerManagedBy(mgr)
	if b == nil {
		t.Fatalf("expected builder")
	}
}

type fakeRuntimeAdapter struct {
	named, forCalled, opts, watch, complete bool
}

func (f *fakeRuntimeAdapter) Named(string) controllerRuntimeAdapter { f.named = true; return f }
func (f *fakeRuntimeAdapter) For(client.Object, ...builder.ForOption) controllerRuntimeAdapter {
	f.forCalled = true
	return f
}
func (f *fakeRuntimeAdapter) WithOptions(controller.Options) controllerRuntimeAdapter {
	f.opts = true
	return f
}
func (f *fakeRuntimeAdapter) WatchesRawSource(source.Source) controllerRuntimeAdapter {
	f.watch = true
	return f
}
func (f *fakeRuntimeAdapter) Complete(reconcile.Reconciler) error { f.complete = true; return nil }

func TestRuntimeControllerBuilderDelegates(t *testing.T) {
	adapter := &fakeRuntimeAdapter{}
	rcb := &runtimeControllerBuilder{adapter: adapter}
	if err := rcb.Named("x").
		For(&v1alpha1.ClusterGPUPool{}).
		WithOptions(controller.Options{}).
		WatchesRawSource(source.Kind(&fakeCache{}, &unstructured.Unstructured{}, nil)).
		Complete(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !(adapter.named && adapter.forCalled && adapter.opts && adapter.watch && adapter.complete) {
		t.Fatalf("runtimeControllerBuilder did not delegate all calls")
	}
}

type fakeDelegate struct {
	named, forCalled, opts, watch, complete bool
}

func (f *fakeDelegate) Named(string) *builder.Builder { f.named = true; return &builder.Builder{} }
func (f *fakeDelegate) For(client.Object, ...builder.ForOption) *builder.Builder {
	f.forCalled = true
	return &builder.Builder{}
}
func (f *fakeDelegate) WithOptions(controller.Options) *builder.Builder {
	f.opts = true
	return &builder.Builder{}
}
func (f *fakeDelegate) WatchesRawSource(source.Source) *builder.Builder {
	f.watch = true
	return &builder.Builder{}
}
func (f *fakeDelegate) Complete(reconcile.Reconciler) error { f.complete = true; return nil }

type nopReconciler struct{}

func (nopReconciler) Reconcile(context.Context, ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func TestBuilderControllerAdapterDelegates(t *testing.T) {
	delegate := &fakeDelegate{}
	adapter := &builderControllerAdapter{delegate: delegate}
	adapter.Named("x")
	adapter.For(&v1alpha1.ClusterGPUPool{})
	adapter.WithOptions(controller.Options{})
	adapter.WatchesRawSource(source.Kind(&fakeCache{}, &unstructured.Unstructured{}, nil))
	if err := adapter.Complete(nopReconciler{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !(delegate.named && delegate.forCalled && delegate.opts && delegate.watch && delegate.complete) {
		t.Fatalf("builderControllerAdapter did not delegate all calls")
	}
}

type errClient struct {
	client.Client
	getErr  error
	listErr error
}

func (c *errClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.getErr
}
func (c *errClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.listErr
}
func (c *errClient) Status() client.StatusWriter { return errStatusWriter{} }

type errStatusWriter struct{}

func (errStatusWriter) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return nil
}
func (errStatusWriter) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return nil
}
func (errStatusWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return nil
}

func TestReconcileGetError(t *testing.T) {
	rec := &Reconciler{
		client: &errClient{getErr: errors.New("boom")},
		log:    logr.Discard(),
		cfg:    config.ControllerConfig{Workers: 1},
	}
	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatalf("expected error from client")
	}
}

func TestRequeueAllPoolsListError(t *testing.T) {
	rec := &Reconciler{
		client: &errClient{listErr: errors.New("list error")},
		log:    testr.New(t),
	}
	if reqs := rec.requeueAllPools(context.Background()); reqs != nil {
		t.Fatalf("expected nil requeue on list error")
	}
}
