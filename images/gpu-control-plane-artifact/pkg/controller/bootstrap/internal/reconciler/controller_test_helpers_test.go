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
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	promdto "github.com/prometheus/client_model/go"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

type fakeManager struct {
	client  client.Client
	scheme  *runtime.Scheme
	log     logr.Logger
	cache   cache.Cache
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

type stubFieldIndexer struct {
	results [][]string
	err     error
}

func (s *stubFieldIndexer) IndexField(_ context.Context, obj client.Object, _ string, extractValue client.IndexerFunc) error {
	switch obj := obj.(type) {
	case *corev1.Pod:
		_ = obj
		s.results = append(s.results, extractValue(&corev1.Pod{Spec: corev1.PodSpec{NodeName: "node-a"}}))
		s.results = append(s.results, extractValue(&corev1.Pod{}))
		s.results = append(s.results, extractValue(&corev1.Node{}))
	case *v1alpha1.GPUDevice:
		_ = obj
		s.results = append(s.results, extractValue(&v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{NodeName: "node-b"}}))
		s.results = append(s.results, extractValue(&v1alpha1.GPUDevice{}))
		s.results = append(s.results, extractValue(&corev1.Node{}))
	default:
		s.results = append(s.results, extractValue(obj))
	}
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
