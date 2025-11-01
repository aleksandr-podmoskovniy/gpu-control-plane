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
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// --- Test fakes ----------------------------------------------------------------

type fakeManager struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

func newFakeManager(c client.Client, scheme *runtime.Scheme) *fakeManager {
	return &fakeManager{client: c, scheme: scheme}
}

func (f *fakeManager) GetClient() client.Client                        { return f.client }
func (f *fakeManager) GetScheme() *runtime.Scheme                      { return f.scheme }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer            { return nil }
func (f *fakeManager) GetHTTPClient() *http.Client                     { return nil }
func (f *fakeManager) GetConfig() *rest.Config                         { return nil }
func (f *fakeManager) GetCache() cache.Cache                           { return nil }
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

type fakeBuilder struct {
	named         string
	forObject     client.Object
	options       controller.Options
	completeErr   error
	completeCalls int
}

func (f *fakeBuilder) Named(name string) controllerBuilder {
	f.named = name
	return f
}

func (f *fakeBuilder) For(obj client.Object, _ ...builder.ForOption) controllerBuilder {
	f.forObject = obj
	return f
}

func (f *fakeBuilder) WithOptions(opts controller.Options) controllerBuilder {
	f.options = opts
	return f
}

func (f *fakeBuilder) Complete(reconcile.Reconciler) error {
	f.completeCalls++
	return f.completeErr
}

type stubPoolHandler struct {
	name   string
	result contracts.Result
	err    error
	calls  int
}

func (s *stubPoolHandler) Name() string { return s.name }

func (s *stubPoolHandler) HandlePool(context.Context, *gpuv1alpha1.GPUPool) (contracts.Result, error) {
	s.calls++
	return s.result, s.err
}

type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return f.err
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

// --- Tests ----------------------------------------------------------------------

func TestNewNormalisesWorkers(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{Workers: 0}, nil)
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers defaulted to 1, got %d", rec.cfg.Workers)
	}
}

func TestSetupWithManagerUsesBuilder(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(client, scheme)

	stub := &fakeBuilder{}
	rec := New(testr.New(t), config.ControllerConfig{Workers: 5}, nil)
	rec.builders = func(ctrl.Manager) controllerBuilder { return stub }

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager failed: %v", err)
	}
	if rec.client != client {
		t.Fatal("manager client not captured")
	}
	if rec.scheme != scheme {
		t.Fatal("manager scheme not captured")
	}
	if stub.named != "gpu-pool-controller" {
		t.Fatalf("unexpected controller name: %s", stub.named)
	}
	if _, ok := stub.forObject.(*gpuv1alpha1.GPUPool); !ok {
		t.Fatalf("expected For GPUPool, got %T", stub.forObject)
	}
	if stub.options.MaxConcurrentReconciles != 5 {
		t.Fatalf("expected workers=5, got %d", stub.options.MaxConcurrentReconciles)
	}
	if stub.options.RecoverPanic == nil || !*stub.options.RecoverPanic {
		t.Fatalf("expected RecoverPanic enabled")
	}
	if stub.options.LogConstructor == nil {
		t.Fatalf("expected LogConstructor configured")
	}
	if stub.options.CacheSyncTimeout != cacheSyncTimeoutDuration {
		t.Fatalf("expected CacheSyncTimeout=%s, got %s", cacheSyncTimeoutDuration, stub.options.CacheSyncTimeout)
	}
	if stub.completeCalls != 1 {
		t.Fatalf("expected Complete invoked once, got %d", stub.completeCalls)
	}
}

func TestSetupWithManagerPropagatesError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.builders = func(ctrl.Manager) controllerBuilder {
		return &fakeBuilder{completeErr: errors.New("builder fail")}
	}
	if err := rec.SetupWithManager(context.Background(), newFakeManager(nil, nil)); err == nil {
		t.Fatal("expected builder error")
	}
}

func TestReconcileAggregatesResults(t *testing.T) {
	scheme := newScheme(t)
	pool := &gpuv1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	handlerA := &stubPoolHandler{name: "a", result: contracts.Result{Requeue: true}}
	handlerB := &stubPoolHandler{name: "b", result: contracts.Result{RequeueAfter: time.Second}}

	rec := New(testr.New(t), config.ControllerConfig{}, []contracts.PoolHandler{handlerA, handlerB})
	rec.client = client
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != time.Second {
		t.Fatalf("unexpected aggregate result: %+v", res)
	}
	if handlerA.calls != 1 || handlerB.calls != 1 {
		t.Fatalf("expected handlers invoked once, got %d/%d", handlerA.calls, handlerB.calls)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := newScheme(t)
	pool := &gpuv1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	handler := &stubPoolHandler{name: "boom", err: errors.New("handler fail")}

	rec := New(testr.New(t), config.ControllerConfig{}, []contracts.PoolHandler{handler})
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatal("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("expected handler called once, got %d", handler.calls)
	}
}

func TestReconcileGetError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = &failingClient{err: errors.New("get fail")}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatal("expected get error")
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReconcileNoHandlers(t *testing.T) {
	scheme := newScheme(t)
	pool := &gpuv1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = client
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestRuntimeControllerBuilderDelegates(t *testing.T) {
	wrapper := &runtimeControllerBuilder{builder: &builder.Builder{}}

	if wrapper.Named("gpupool") != wrapper {
		t.Fatal("Named should return wrapper")
	}
	if wrapper.For(&gpuv1alpha1.GPUPool{}) != wrapper {
		t.Fatal("For should return wrapper")
	}
	if wrapper.WithOptions(controller.Options{MaxConcurrentReconciles: 2}) != wrapper {
		t.Fatal("WithOptions should return wrapper")
	}
	if err := wrapper.Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
		return reconcile.Result{}, nil
	})); err == nil {
		t.Fatal("expected Complete to fail without manager")
	}
}

func TestNewControllerManagedByReturnsBuilder(t *testing.T) {
	if b := newControllerManagedBy(nil); b == nil {
		t.Fatal("expected builder wrapper")
	}
}

func TestReconcileWrapsAPIError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = &failingClient{err: apierrors.NewConflict(schema.GroupResource{Group: gpuv1alpha1.GroupVersion.Group, Resource: "gpupools"}, "pool", errors.New("boom"))}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatal("expected API error")
	}
}
