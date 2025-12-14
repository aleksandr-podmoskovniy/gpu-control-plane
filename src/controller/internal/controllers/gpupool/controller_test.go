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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/indexer"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

// --- Test fakes ----------------------------------------------------------------

type fakeManager struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
	cache  cache.Cache
	indexer client.FieldIndexer
}

func newFakeManager(c client.Client, scheme *runtime.Scheme) *fakeManager {
	return &fakeManager{client: c, scheme: scheme}
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

type fakeBuilder struct {
	named          string
	forObject      client.Object
	options        controller.Options
	watchedSources []source.Source
	completeErr    error
	completeCalls  int
}

func (f *fakeBuilder) Named(name string) controllerbuilder.Builder {
	f.named = name
	return f
}

func (f *fakeBuilder) For(obj client.Object, _ ...builder.ForOption) controllerbuilder.Builder {
	f.forObject = obj
	return f
}

func (f *fakeBuilder) Owns(client.Object, ...builder.OwnsOption) controllerbuilder.Builder {
	return f
}

func (f *fakeBuilder) WatchesRawSource(src source.Source) controllerbuilder.Builder {
	f.watchedSources = append(f.watchedSources, src)
	return f
}

func (f *fakeBuilder) WithOptions(opts controller.Options) controllerbuilder.Builder {
	f.options = opts
	return f
}

func (f *fakeBuilder) Complete(reconcile.Reconciler) error {
	f.completeCalls++
	return f.completeErr
}

type fakeCache struct{ cache.Cache }

type stubPoolHandler struct {
	name   string
	result contracts.Result
	err    error
	calls  int
}

func (s *stubPoolHandler) Name() string { return s.name }

func (s *stubPoolHandler) HandlePool(context.Context, *v1alpha1.GPUPool) (contracts.Result, error) {
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

type failingListClient struct {
	client.Client
	err error
}

func (f *failingListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return f.err
}

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

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

// --- Tests ----------------------------------------------------------------------

func TestNewNormalisesWorkers(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{Workers: 0}, nil, nil)
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers defaulted to 1, got %d", rec.cfg.Workers)
	}
}

func TestSetupWithManagerUsesBuilder(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(client, scheme)

	stub := &fakeBuilder{}
	rec := New(testr.New(t), config.ControllerConfig{Workers: 5}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return stub }

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
	if _, ok := stub.forObject.(*v1alpha1.GPUPool); !ok {
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

func TestSetupWithManagerIndexesFieldsWhenIndexerProvided(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	idx := &stubFieldIndexer{}
	mgr := newFakeManager(cl, scheme)
	mgr.indexer = idx

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return &fakeBuilder{} }

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager failed: %v", err)
	}
	if idx.calls != 4 {
		t.Fatalf("expected 4 index registrations, got %d", idx.calls)
	}
}

func TestSetupWithManagerPropagatesIndexerError(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	for _, failAt := range []int{1, 2, 3, 4} {
		t.Run("failAt="+time.Duration(failAt).String(), func(t *testing.T) {
			idx := &stubFieldIndexer{err: errors.New("index fail"), failAt: failAt}
			mgr := newFakeManager(cl, scheme)
			mgr.indexer = idx

			rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
			rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return &fakeBuilder{} }

			if err := rec.SetupWithManager(context.Background(), mgr); err == nil {
				t.Fatalf("expected indexer error")
			}
		})
	}
}

func TestSetupWithManagerAddsModuleConfigWatch(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(client, scheme)
	mgr.cache = &fakeCache{}

	stub := &fakeBuilder{}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return stub }

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager failed: %v", err)
	}

	if len(stub.watchedSources) != 2 {
		t.Fatalf("expected module config + device watchers, got %d", len(stub.watchedSources))
	}
}

func TestSetupWithManagerWithoutModuleWatcherFactory(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.cache = &fakeCache{}

	stub := &fakeBuilder{}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return stub }
	rec.moduleWatcherFactory = nil

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager failed: %v", err)
	}
	if len(stub.watchedSources) != 1 {
		t.Fatalf("expected only device watcher, got %d", len(stub.watchedSources))
	}
}

func TestSetupWithManagerPropagatesError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder {
		return &fakeBuilder{completeErr: errors.New("builder fail")}
	}
	if err := rec.SetupWithManager(context.Background(), newFakeManager(nil, nil)); err == nil {
		t.Fatal("expected builder error")
	}
}

func TestReconcileAggregatesResults(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(pool).
		Build()

	handlerA := &stubPoolHandler{name: "a", result: contracts.Result{Requeue: true}}
	handlerB := &stubPoolHandler{name: "b", result: contracts.Result{RequeueAfter: time.Second}}

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.PoolHandler{handlerA, handlerB})
	rec.client = client
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}})
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

func TestRequeueAllPools(t *testing.T) {
	scheme := newScheme(t)
	poolA := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns-a"}}
	poolB := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-b", Namespace: "ns-b"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(poolA, poolB).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	requests := rec.requeueAllPools(context.Background())
	if len(requests) != 2 {
		t.Fatalf("expected two requeue requests, got %#v", requests)
	}
	expected := map[string]struct{}{"ns-a/pool-a": {}, "ns-b/pool-b": {}}
	for _, req := range requests {
		key := req.Namespace + "/" + req.Name
		if _, ok := expected[key]; !ok {
			t.Fatalf("unexpected request %v", req)
		}
	}
}

func TestMapDeviceRequeuesPools(t *testing.T) {
	scheme := newScheme(t)
	poolA := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns-a"}}
	poolB := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-b", Namespace: "ns-b"}}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&v1alpha1.GPUPool{}, indexer.GPUPoolNameField, func(obj client.Object) []string {
			pool, ok := obj.(*v1alpha1.GPUPool)
			if !ok || pool.Name == "" {
				return nil
			}
			return []string{pool.Name}
		}).
		WithObjects(poolA, poolB).
		Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	// Unassigned device should not trigger requeue.
	if devReqs := rec.mapDevice(context.Background(), &v1alpha1.GPUDevice{}); len(devReqs) != 0 {
		t.Fatalf("expected unassigned device to be ignored, got %d requests", len(devReqs))
	}

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{assignmentAnnotation: "pool-b"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool-a"},
		},
	}
	reqs := rec.mapDevice(context.Background(), dev)
	if len(reqs) != 2 {
		t.Fatalf("expected two reconcile requests, got %#v", reqs)
	}

	got := map[string]struct{}{}
	for _, req := range reqs {
		got[req.Namespace+"/"+req.Name] = struct{}{}
	}
	if _, ok := got["ns-a/pool-a"]; !ok {
		t.Fatalf("expected request for ns-a/pool-a, got %#v", reqs)
	}
	if _, ok := got["ns-b/pool-b"]; !ok {
		t.Fatalf("expected request for ns-b/pool-b, got %#v", reqs)
	}
}

func TestMapDeviceNilDeviceReturnsNil(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	if reqs := rec.mapDevice(context.Background(), nil); reqs != nil {
		t.Fatalf("expected nil requests for nil device, got %#v", reqs)
	}
}

func TestMapDeviceReturnsNamespacedPoolRefWhenNoTargetPools(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	dev := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool-a", Namespace: "ns-a"},
		},
	}

	reqs := rec.mapDevice(context.Background(), dev)
	if len(reqs) != 1 || reqs[0].NamespacedName != (types.NamespacedName{Name: "pool-a", Namespace: "ns-a"}) {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
}

func TestMapDeviceIgnoresEmptyPoolRefName(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingListClient{err: errors.New("should not list")}

	dev := &v1alpha1.GPUDevice{
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: ""},
		},
	}

	if reqs := rec.mapDevice(context.Background(), dev); len(reqs) != 0 {
		t.Fatalf("expected empty requests for poolRef with empty name, got %#v", reqs)
	}
}

func TestMapDeviceListFailureIsIgnored(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingListClient{err: errors.New("list fail")}

	dev := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool-a"},
		},
	}

	if reqs := rec.mapDevice(context.Background(), dev); len(reqs) != 0 {
		t.Fatalf("expected no requests on list failure, got %#v", reqs)
	}
}

func TestMapModuleConfigRequeuesPools(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(pool).
		Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	reqs := rec.mapModuleConfig(context.Background(), &unstructured.Unstructured{})
	if len(reqs) != 1 || reqs[0].Name != "pool-a" || reqs[0].Namespace != "ns" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
}

func TestMapModuleConfigSkipsWhenModuleDisabled(t *testing.T) {
	store := config.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings})
	rec := New(testr.New(t), config.ControllerConfig{}, store, nil)

	if reqs := rec.mapModuleConfig(context.Background(), &unstructured.Unstructured{}); len(reqs) != 0 {
		t.Fatalf("expected no requests when module is disabled, got %#v", reqs)
	}
}

func TestDevicePredicates(t *testing.T) {
	p := devicePredicates()

	if p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: nil}) {
		t.Fatalf("expected create predicate to ignore nil device")
	}
	if !p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{assignmentAnnotation: "pool"}}}}) {
		t.Fatalf("expected create predicate to trigger on assignment")
	}
	if !p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"}}}}) {
		t.Fatalf("expected create predicate to trigger on poolRef")
	}

	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: nil, ObjectNew: &v1alpha1.GPUDevice{}}) {
		t.Fatalf("expected update predicate to pass through nil old")
	}
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: &v1alpha1.GPUDevice{}, ObjectNew: nil}) {
		t.Fatalf("expected update predicate to pass through nil new")
	}

	base := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{assignmentAnnotation: "pool"}}}
	same := base.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: base, ObjectNew: same}) {
		t.Fatalf("expected no reconcile when device is unchanged")
	}
	changed := base.DeepCopy()
	changed.Annotations[assignmentAnnotation] = "other"
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: base, ObjectNew: changed}) {
		t.Fatalf("expected reconcile when device assignment changes")
	}

	if !p.Delete(event.TypedDeleteEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{}}) {
		t.Fatalf("expected delete predicate to trigger")
	}
	if p.Generic(event.TypedGenericEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{}}) {
		t.Fatalf("expected generic predicate to be ignored")
	}
}

func TestDeviceChangedDetectsRelevantFields(t *testing.T) {
	base := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{assignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:    v1alpha1.GPUDeviceStateAssigned,
			NodeName: "node",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	same := base.DeepCopy()
	if deviceChanged(base, same) {
		t.Fatalf("expected no change for identical device")
	}

	changed := base.DeepCopy()
	changed.Annotations[assignmentAnnotation] = "other"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected annotation change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.State = v1alpha1.GPUDeviceStateReady
	if !deviceChanged(base, changed) {
		t.Fatalf("expected state change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.NodeName = "node-2"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected nodeName change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.PoolRef = nil
	if !deviceChanged(base, changed) {
		t.Fatalf("expected poolRef removal to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.PoolRef.Name = "pool-2"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected poolRef name change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.PoolRef.Namespace = "ns"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected poolRef namespace change to be detected")
	}
}

func TestRequeueAllPoolsHandlesError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingListClient{err: errors.New("list fail")}

	if res := rec.requeueAllPools(context.Background()); len(res) != 0 {
		t.Fatalf("expected empty result on error, got %#v", res)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(pool).
		Build()

	handler := &stubPoolHandler{name: "boom", err: errors.New("handler fail")}

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.PoolHandler{handler})
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatal("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("expected handler called once, got %d", handler.calls)
	}
}

func TestReconcileGetError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: errors.New("get fail")}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatal("expected get error")
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "missing"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReconcileNoHandlers(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(pool).
		Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestReconcileWrapsAPIError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpupools"}, "pool", errors.New("boom"))}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatal("expected API error")
	}
}
