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

package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	promdto "github.com/prometheus/client_model/go"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
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
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	bootstrapmeta "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
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
func (f *fakeManager) GetRESTMapper() apimeta.RESTMapper               { return nil }
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

func (f *fakeBuilder) Owns(client.Object, ...builder.OwnsOption) controllerbuilder.Builder { return f }

func (f *fakeBuilder) WithOptions(opts controller.Options) controllerbuilder.Builder {
	f.options = opts
	return f
}

func (f *fakeBuilder) WatchesRawSource(src source.Source) controllerbuilder.Builder {
	f.watchedSources = append(f.watchedSources, src)
	return f
}

func (f *fakeBuilder) Complete(reconcile.Reconciler) error {
	f.completeCalls++
	return f.completeErr
}

func labelsMatch(metric *promdto.Metric, expected map[string]string) bool {
	for name, want := range expected {
		found := false
		for _, pair := range metric.Label {
			if pair.GetName() != name {
				continue
			}
			found = true
			if pair.GetValue() != want {
				return false
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func findMetric(t *testing.T, name string, labels map[string]string) (*promdto.Metric, bool) {
	t.Helper()

	families, err := crmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if labelsMatch(metric, labels) {
				return metric, true
			}
		}
		return nil, false
	}

	return nil, false
}

func counterValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Counter == nil {
		return 0, false
	}
	return metric.Counter.GetValue(), true
}

func gaugeValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Gauge == nil {
		return 0, false
	}
	return metric.Gauge.GetValue(), true
}

type fakeCache struct{ cache.Cache }

type stubBootstrapHandler struct {
	name   string
	result contracts.Result
	err    error
	calls  int
}

func (s *stubBootstrapHandler) Name() string { return s.name }

func (s *stubBootstrapHandler) HandleNode(context.Context, *v1alpha1.GPUNodeState) (contracts.Result, error) {
	s.calls++
	return s.result, s.err
}

type clientAwareBootstrapHandler struct {
	client client.Client
}

func (h *clientAwareBootstrapHandler) Name() string { return "client-aware" }

func (h *clientAwareBootstrapHandler) HandleNode(context.Context, *v1alpha1.GPUNodeState) (contracts.Result, error) {
	return contracts.Result{}, nil
}

func (h *clientAwareBootstrapHandler) SetClient(cl client.Client) {
	h.client = cl
}

type stubValidator struct {
	statusCalls int
}

func (s *stubValidator) Status(context.Context, string) (validation.Result, error) {
	s.statusCalls++
	return validation.Result{}, nil
}

type capturingValidator struct {
	statusCalls int
	nodeName    string
	result      validation.Result
	err         error
}

func (v *capturingValidator) Status(_ context.Context, nodeName string) (validation.Result, error) {
	v.statusCalls++
	v.nodeName = nodeName
	return v.result, v.err
}

type statusChangingHandler struct{}

func (statusChangingHandler) Name() string { return "status-changer" }

func (statusChangingHandler) HandleNode(_ context.Context, inventory *v1alpha1.GPUNodeState) (contracts.Result, error) {
	inventory.Status.Conditions = append(inventory.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})
	return contracts.Result{}, nil
}

type statusReadingHandler struct {
	present bool
	status  validation.Result
}

func (h *statusReadingHandler) Name() string { return "status-reader" }

func (h *statusReadingHandler) HandleNode(ctx context.Context, _ *v1alpha1.GPUNodeState) (contracts.Result, error) {
	h.status, h.present = validation.StatusFromContext(ctx)
	return contracts.Result{}, nil
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
	results [][]string
	err     error
}

func (s *stubFieldIndexer) IndexField(_ context.Context, _ client.Object, _ string, extractValue client.IndexerFunc) error {
	s.results = append(s.results, extractValue(&corev1.Pod{Spec: corev1.PodSpec{NodeName: "node-a"}}))
	s.results = append(s.results, extractValue(&corev1.Pod{}))
	s.results = append(s.results, extractValue(&corev1.Node{}))
	return s.err
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

// --- Tests ---------------------------------------------------------------------

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
	rec := New(testr.New(t), config.ControllerConfig{Workers: 3}, nil, nil)
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
	if stub.named != "gpu-bootstrap-controller" {
		t.Fatalf("unexpected controller name: %s", stub.named)
	}
	if _, ok := stub.forObject.(*v1alpha1.GPUNodeState); !ok {
		t.Fatalf("expected For GPUNodeState, got %T", stub.forObject)
	}
	if stub.options.MaxConcurrentReconciles != 3 {
		t.Fatalf("expected workers=3, got %d", stub.options.MaxConcurrentReconciles)
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

func TestSetupWithManagerIndexesPodsWhenIndexerProvided(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.indexer = &stubFieldIndexer{}

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return &fakeBuilder{} }

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager failed: %v", err)
	}
	idx := mgr.indexer.(*stubFieldIndexer)
	if len(idx.results) != 3 {
		t.Fatalf("expected 3 extractValue calls, got %d", len(idx.results))
	}
	if len(idx.results[0]) != 1 || idx.results[0][0] != "node-a" {
		t.Fatalf("unexpected extractValue result: %#v", idx.results[0])
	}
	if idx.results[1] != nil || idx.results[2] != nil {
		t.Fatalf("expected nil extractValue results for non-matching objects, got %#v", idx.results[1:])
	}
}

func TestSetupWithManagerPropagatesIndexerError(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(cl, scheme)
	mgr.indexer = &stubFieldIndexer{err: errors.New("index fail")}

	rec := New(testr.New(t), config.ControllerConfig{Workers: 1}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder { return &fakeBuilder{} }

	if err := rec.SetupWithManager(context.Background(), mgr); err == nil {
		t.Fatalf("expected indexer error")
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

	if len(stub.watchedSources) == 0 {
		t.Fatalf("expected module config watcher registered, watchedSources=%#v", stub.watchedSources)
	}
}

func TestModuleWatcherFactoryRegistersSource(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	stub := &fakeBuilder{}

	builder := rec.moduleWatcherFactory(&fakeCache{}, stub)
	if builder != stub {
		t.Fatal("expected moduleWatcherFactory to return original builder")
	}
	if len(stub.watchedSources) == 0 {
		t.Fatal("expected module watcher to be registered")
	}
}

func TestWorkloadWatcherFactoryRegistersSource(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	stub := &fakeBuilder{}

	builder := rec.workloadWatcherFactory(&fakeCache{}, stub)
	if builder != stub {
		t.Fatal("expected workloadWatcherFactory to return original builder")
	}
	if len(stub.watchedSources) == 0 {
		t.Fatal("expected workload watcher to register source")
	}
}

func TestSetupWithManagerPropagatesError(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(client, scheme)

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.builders = func(ctrl.Manager) controllerbuilder.Builder {
		return &fakeBuilder{completeErr: errors.New("builder fail")}
	}

	if err := rec.SetupWithManager(context.Background(), mgr); err == nil {
		t.Fatal("expected builder error")
	}
}

func TestMapWorkloadPodToInventory(t *testing.T) {
	apps := bootstrapmeta.ComponentAppNames()
	if len(apps) == 0 {
		t.Fatal("component app names must be defined")
	}
	validApp := apps[0]

	tests := []struct {
		name string
		pod  *corev1.Pod
		want int
	}{
		{name: "nil", pod: nil, want: 0},
		{name: "no node", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: bootstrapmeta.WorkloadsNamespace, Labels: map[string]string{"app": validApp}}}, want: 0},
		{name: "no labels", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: bootstrapmeta.WorkloadsNamespace}, Spec: corev1.PodSpec{NodeName: "node-a"}}, want: 0},
		{name: "other namespace", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Labels: map[string]string{"app": validApp}}, Spec: corev1.PodSpec{NodeName: "node-a"}}, want: 0},
		{name: "unknown component", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: bootstrapmeta.WorkloadsNamespace, Labels: map[string]string{"app": "other"}}, Spec: corev1.PodSpec{NodeName: "node-a"}}, want: 0},
		{name: "scheduled managed pod", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: bootstrapmeta.WorkloadsNamespace, Labels: map[string]string{"app": validApp}}, Spec: corev1.PodSpec{NodeName: "node-a"}}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapWorkloadPodToInventory(context.Background(), tt.pod)
			if len(got) != tt.want {
				t.Fatalf("expected %d requests, got %d", tt.want, len(got))
			}
			if tt.want > 0 && got[0].Name != "node-a" {
				t.Fatalf("expected request for node-a, got %s", got[0].Name)
			}
		})
	}
}

func TestRequeueAllInventoriesHandlesNilClient(t *testing.T) {
	rec := &Reconciler{}
	if req := rec.requeueAllInventories(context.Background()); req != nil {
		t.Fatalf("expected nil requests when client is not set, got %d", len(req))
	}
}

func TestInjectClientAssignsOnlyHandlersWithSetter(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	withSetter := &clientAwareBootstrapHandler{}
	withoutSetter := &stubBootstrapHandler{name: "plain"}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.BootstrapHandler{withSetter, withoutSetter})
	rec.client = cl

	rec.injectClient()

	if withSetter.client != cl {
		t.Fatal("expected handler with SetClient to receive client")
	}
}

func TestInjectClientNoClient(t *testing.T) {
	withSetter := &clientAwareBootstrapHandler{}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.BootstrapHandler{withSetter})
	rec.client = nil
	rec.injectClient()
	if withSetter.client != nil {
		t.Fatal("expected handler to remain nil when controller client is nil")
	}
}

func TestReconcileAggregatesResults(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	handlerA := &stubBootstrapHandler{name: "a", result: contracts.Result{Requeue: true}}
	handlerB := &stubBootstrapHandler{name: "b", result: contracts.Result{RequeueAfter: time.Second}}

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.BootstrapHandler{handlerA, handlerB})
	rec.client = client
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}})
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

func TestRequeueAllInventories(t *testing.T) {
	scheme := newScheme(t)
	invA := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
	}
	invB := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-b"},
	}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(invA, invB).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	requests := rec.requeueAllInventories(context.Background())
	if len(requests) != 2 {
		t.Fatalf("expected two requeue requests, got %#v", requests)
	}
	expected := map[string]struct{}{"node-a": {}, "node-b": {}}
	for _, req := range requests {
		if _, ok := expected[req.Name]; !ok {
			t.Fatalf("unexpected request %v", req)
		}
	}
}

func TestMapModuleConfigRequeuesInventories(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
	}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	reqs := rec.mapModuleConfig(context.Background(), &unstructured.Unstructured{})
	if len(reqs) != 1 || reqs[0].Name != "node-a" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
}

func TestMapModuleConfigSkipsWhenModuleDisabled(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
	}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()
	store := config.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings})

	rec := New(testr.New(t), config.ControllerConfig{}, store, nil)
	rec.client = client

	if reqs := rec.mapModuleConfig(context.Background(), &unstructured.Unstructured{}); len(reqs) != 0 {
		t.Fatalf("expected no requests when module is disabled, got %#v", reqs)
	}
}

func TestRequeueAllInventoriesHandlesError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingListClient{err: errors.New("list fail")}

	if res := rec.requeueAllInventories(context.Background()); len(res) != 0 {
		t.Fatalf("expected empty result on error, got %#v", res)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	handlerName := "boom-" + t.Name()
	handler := &stubBootstrapHandler{name: handlerName, err: errors.New("handler fail")}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.BootstrapHandler{handler})
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatal("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("expected handler called once, got %d", handler.calls)
	}
	if v, ok := counterValue(t, cpmetrics.BootstrapHandlerErrorsTotal, map[string]string{"handler": handlerName}); !ok || v != 1 {
		t.Fatalf("expected handler error metric incremented, got %f", v)
	}
}

func TestReconcileSkipsWhenModuleDisabled(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()
	store := config.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings})

	rec := New(testr.New(t), config.ControllerConfig{}, store, nil)
	rec.client = client
	rec.scheme = scheme
	rec.validator = &stubValidator{}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := rec.validator.(*stubValidator)
	if val.statusCalls != 0 {
		t.Fatalf("expected validator not to be called, got status=%d", val.statusCalls)
	}
}

func TestReconcileGetError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: errors.New("get fail")}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatal("expected get error")
	}
}

func TestUpdateBootstrapMetricsSetsPhaseAndConditions(t *testing.T) {
	nodeName := "node-metrics-update"
	rec := &Reconciler{}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: nodeName},
	}
	apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionReadyForPooling, Status: metav1.ConditionTrue})
	rec.updateBootstrapMetrics(nodeName, "Validating", inventory)

	if v, ok := gaugeValue(t, cpmetrics.BootstrapNodePhaseMetric, map[string]string{"node": nodeName, "phase": "Ready"}); !ok || v != 1 {
		t.Fatalf("expected phase gauge to be set, got %f", v)
	}
	if v, ok := gaugeValue(t, cpmetrics.BootstrapConditionMetric, map[string]string{"node": nodeName, "condition": conditionReadyForPooling}); !ok || v != 1 {
		t.Fatalf("expected condition gauge to be set, got %f", v)
	}
}

func TestClearBootstrapMetricsRemovesValues(t *testing.T) {
	nodeName := "node-metrics-clear"
	rec := &Reconciler{}
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: nodeName},
	}
	apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionReadyForPooling, Status: metav1.ConditionTrue})
	rec.updateBootstrapMetrics(nodeName, "Validating", inventory)

	if _, ok := findMetric(t, cpmetrics.BootstrapNodePhaseMetric, map[string]string{"node": nodeName, "phase": "Ready"}); !ok {
		t.Fatalf("expected phase gauge populated")
	}
	if _, ok := findMetric(t, cpmetrics.BootstrapConditionMetric, map[string]string{"node": nodeName, "condition": conditionReadyForPooling}); !ok {
		t.Fatalf("expected condition gauge populated")
	}

	rec.clearBootstrapMetrics(nodeName)

	if _, ok := findMetric(t, cpmetrics.BootstrapNodePhaseMetric, map[string]string{"node": nodeName, "phase": "Ready"}); ok {
		t.Fatalf("expected phase gauge cleared")
	}
	if _, ok := findMetric(t, cpmetrics.BootstrapConditionMetric, map[string]string{"node": nodeName, "condition": conditionReadyForPooling}); ok {
		t.Fatalf("expected condition gauge cleared")
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReconcileNoHandlers(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client
	rec.scheme = scheme

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestReconcilePersistsStatusChanges(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		WithStatusSubresource(&v1alpha1.GPUNodeState{}).
		Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.BootstrapHandler{statusChangingHandler{}})
	rec.client = client
	rec.scheme = scheme

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.GPUNodeState{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node"}, updated); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if len(updated.Status.Conditions) != 1 || updated.Status.Conditions[0].Type != "Ready" {
		t.Fatalf("expected condition persisted, got %+v", updated.Status.Conditions)
	}
}

func TestEffectiveBootstrapPhaseDefaultAndDisabled(t *testing.T) {
	inventory := &v1alpha1.GPUNodeState{}
	if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
		t.Fatalf("expected default phase Validating, got %s", phase)
	}
	inventory.Status.Conditions = []metav1.Condition{{Type: conditionManagedDisabled, Status: metav1.ConditionTrue}}
	if phase := effectiveBootstrapPhase(inventory); phase != "Disabled" {
		t.Fatalf("expected phase Disabled when managed-disabled, got %s", phase)
	}
}

func TestEffectiveBootstrapPhaseVariants(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionReadyForPooling, Status: metav1.ConditionTrue})
		if phase := effectiveBootstrapPhase(inventory); phase != "Ready" {
			t.Fatalf("expected phase Ready, got %s", phase)
		}
	})

	t.Run("validating-driver", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionDriverMissing, Status: metav1.ConditionTrue})
		if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
			t.Fatalf("expected phase Validating, got %s", phase)
		}
	})

	t.Run("validating-toolkit", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionToolkitMissing, Status: metav1.ConditionTrue})
		if phase := effectiveBootstrapPhase(inventory); phase != "Validating" {
			t.Fatalf("expected phase Validating, got %s", phase)
		}
	})

	t.Run("monitoring-missing", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionMonitoringMissing, Status: metav1.ConditionTrue})
		if phase := effectiveBootstrapPhase(inventory); phase != "Monitoring" {
			t.Fatalf("expected phase Monitoring, got %s", phase)
		}
	})

	t.Run("monitoring-gfd-false", func(t *testing.T) {
		inventory := &v1alpha1.GPUNodeState{}
		apimeta.SetStatusCondition(&inventory.Status.Conditions, metav1.Condition{Type: conditionGFDReady, Status: metav1.ConditionFalse})
		if phase := effectiveBootstrapPhase(inventory); phase != "Monitoring" {
			t.Fatalf("expected phase Monitoring, got %s", phase)
		}
	})
}

func TestReconcileLogsValidatorStatusErrorsAndUsesSpecNodeName(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-validator-error"},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "node-from-spec"},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()

	validator := &capturingValidator{err: errors.New("validator status failed")}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = cl
	rec.scheme = scheme
	rec.validator = validator

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: inventory.Name}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if validator.statusCalls != 1 || validator.nodeName != "node-from-spec" {
		t.Fatalf("expected validator called with node-from-spec once, got calls=%d node=%q", validator.statusCalls, validator.nodeName)
	}
}

func TestReconcilePassesValidatorStatusToHandlers(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-validator-status"},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()

	validator := &capturingValidator{result: validation.Result{DriverReady: true}}
	handler := &statusReadingHandler{}

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []contracts.BootstrapHandler{handler})
	rec.client = cl
	rec.scheme = scheme
	rec.validator = validator

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: inventory.Name}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handler.present || !handler.status.DriverReady {
		t.Fatalf("expected validator status propagated to handler, got present=%t status=%+v", handler.present, handler.status)
	}
}

func TestReconcileWrapsAPIError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpunodestates"}, "node", errors.New("boom"))}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatal("expected API error")
	}
}
