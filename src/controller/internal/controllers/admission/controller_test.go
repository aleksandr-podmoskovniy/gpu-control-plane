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

package admission

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type fakeManager struct {
	client   client.Client
	scheme   *runtime.Scheme
	elected  chan struct{}
	fieldIdx client.FieldIndexer
	cache    cache.Cache
	log      logr.Logger
}

func newFakeManager(c client.Client, scheme *runtime.Scheme) *fakeManager {
	return &fakeManager{
		client:  c,
		scheme:  scheme,
		elected: make(chan struct{}),
	}
}

func (f *fakeManager) GetClient() client.Client                        { return f.client }
func (f *fakeManager) GetScheme() *runtime.Scheme                      { return f.scheme }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer            { return f.fieldIdx }
func (f *fakeManager) GetHTTPClient() *http.Client                     { return nil }
func (f *fakeManager) GetConfig() *rest.Config                         { return nil }
func (f *fakeManager) GetCache() cache.Cache                           { return f.cache }
func (f *fakeManager) GetEventRecorderFor(string) record.EventRecorder { return nil }
func (f *fakeManager) GetRESTMapper() meta.RESTMapper                  { return nil }
func (f *fakeManager) GetAPIReader() client.Reader                     { return nil }
func (f *fakeManager) Start(context.Context) error                     { return nil }
func (f *fakeManager) Add(manager.Runnable) error                      { return nil }
func (f *fakeManager) Elected() <-chan struct{}                        { return f.elected }
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

type stubAdmissionHandler struct {
	name   string
	result contracts.Result
	err    error
	calls  int
}

func (s *stubAdmissionHandler) Name() string { return s.name }

func (s *stubAdmissionHandler) SyncPool(context.Context, *gpuv1alpha1.GPUPool) (contracts.Result, error) {
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

func TestNewNormalisesWorkers(t *testing.T) {
	r := New(testr.New(t), config.ControllerConfig{Workers: 0}, nil)
	if r.cfg.Workers != 1 {
		t.Fatalf("expected workers defaulted to 1, got %d", r.cfg.Workers)
	}
}

func TestSetupWithManagerUsesBuilder(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)

	builderStub := &fakeBuilder{}
	rec := New(testr.New(t), config.ControllerConfig{Workers: 3}, nil)
	rec.builders = func(ctrl.Manager) controllerBuilder { return builderStub }

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager failed: %v", err)
	}
	if rec.client != cl {
		t.Fatal("manager client not captured")
	}
	if rec.scheme != scheme {
		t.Fatal("manager scheme not captured")
	}
	if builderStub.named != "gpu-admission-controller" {
		t.Fatalf("unexpected builder name: %s", builderStub.named)
	}
	if _, ok := builderStub.forObject.(*gpuv1alpha1.GPUPool); !ok {
		t.Fatalf("expected GPUPool For object, got %T", builderStub.forObject)
	}
	if builderStub.options.MaxConcurrentReconciles != 3 {
		t.Fatalf("expected workers=3, got %d", builderStub.options.MaxConcurrentReconciles)
	}
	if builderStub.options.RecoverPanic == nil || !*builderStub.options.RecoverPanic {
		t.Fatalf("expected RecoverPanic enabled")
	}
	if builderStub.options.LogConstructor == nil {
		t.Fatalf("expected LogConstructor configured")
	}
	if builderStub.options.CacheSyncTimeout != cacheSyncTimeoutDuration {
		t.Fatalf("expected CacheSyncTimeout=%s, got %s", cacheSyncTimeoutDuration, builderStub.options.CacheSyncTimeout)
	}
	if builderStub.completeCalls != 1 {
		t.Fatalf("expected Complete invoked once, got %d", builderStub.completeCalls)
	}
}

func TestSetupWithManagerPropagatesBuilderError(t *testing.T) {
	scheme := newScheme(t)
	mgr := newFakeManager(nil, scheme)

	builderStub := &fakeBuilder{completeErr: errors.New("boom")}
	rec := New(testr.New(t), config.ControllerConfig{Workers: 2}, nil)
	rec.builders = func(ctrl.Manager) controllerBuilder { return builderStub }

	if err := rec.SetupWithManager(context.Background(), mgr); err == nil {
		t.Fatal("expected builder error to propagate")
	}
}

func TestReconcileSuccessAggregatesResults(t *testing.T) {
	scheme := newScheme(t)
	pool := &gpuv1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	handlerA := &stubAdmissionHandler{name: "a", result: contracts.Result{Requeue: true}}
	handlerB := &stubAdmissionHandler{name: "b", result: contracts.Result{RequeueAfter: time.Second}}

	rec := New(testr.New(t), config.ControllerConfig{}, []contracts.AdmissionHandler{handlerA, handlerB})
	rec.client = cl
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

func TestReconcileHandlerErrorStopsProcessing(t *testing.T) {
	scheme := newScheme(t)
	pool := &gpuv1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	handlerA := &stubAdmissionHandler{name: "a"}
	handlerB := &stubAdmissionHandler{name: "b", err: errors.New("fail")}
	handlerC := &stubAdmissionHandler{name: "c"}

	rec := New(testr.New(t), config.ControllerConfig{}, []contracts.AdmissionHandler{handlerA, handlerB, handlerC})
	rec.client = cl
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatal("expected handler error to propagate")
	}
	if handlerC.calls != 0 {
		t.Fatalf("expected handlers after failure not invoked, got %d", handlerC.calls)
	}
}

func TestReconcileHandlesGetErrors(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = &failingClient{err: errors.New("get failed")}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}}); err == nil {
		t.Fatal("expected get error")
	}
}

func TestReconcileIgnoresNotFound(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = cl
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing"}}); err != nil {
		t.Fatalf("expected no error on not found, got %v", err)
	}
}

func TestReconcileWithoutHandlers(t *testing.T) {
	scheme := newScheme(t)
	pool := &gpuv1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil)
	rec.client = cl
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestRuntimeControllerBuilderDelegates(t *testing.T) {
	raw := &builder.Builder{}
	wrapper := &runtimeControllerBuilder{builder: raw}

	if wrapper.Named("admission") != wrapper {
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
		t.Fatal("expected Complete to fail with nil manager")
	}
}

func TestNewControllerManagedByCreatesWrapper(t *testing.T) {
	if b := newControllerManagedBy(nil); b == nil {
		t.Fatal("expected builder wrapper")
	}
}
