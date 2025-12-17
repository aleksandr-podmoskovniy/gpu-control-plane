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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"k8s.io/apimachinery/pkg/api/meta"
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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
)

type fakeManager struct {
	client client.Client
	scheme *runtime.Scheme
	cache  cache.Cache
	log    logr.Logger
}

func newFakeManager(c client.Client, scheme *runtime.Scheme) *fakeManager {
	return &fakeManager{client: c, scheme: scheme, log: logr.Discard()}
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

type fakeBuilder struct {
	named    string
	forObj   client.Object
	options  controller.Options
	complete int
	err      error
}

func (f *fakeBuilder) Named(name string) controllerbuilder.Builder {
	f.named = name
	return f
}

func (f *fakeBuilder) For(obj client.Object, _ ...builder.ForOption) controllerbuilder.Builder {
	f.forObj = obj
	return f
}

func (f *fakeBuilder) Owns(client.Object, ...builder.OwnsOption) controllerbuilder.Builder { return f }

func (f *fakeBuilder) WatchesRawSource(source.Source) controllerbuilder.Builder { return f }

func (f *fakeBuilder) WithOptions(opts controller.Options) controllerbuilder.Builder {
	f.options = opts
	return f
}

func (f *fakeBuilder) Complete(reconcile.Reconciler) error {
	f.complete++
	return f.err
}

func newContext() context.Context {
	return ctrllog.IntoContext(context.Background(), logr.Discard())
}

func newModuleConfigScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(ModuleConfigGVK, &unstructured.Unstructured{})
	listGVK := schema.GroupVersionKind{Group: ModuleConfigGVK.Group, Version: ModuleConfigGVK.Version, Kind: ModuleConfigGVK.Kind + "List"}
	scheme.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	return scheme
}

func TestNewRequiresStore(t *testing.T) {
	if _, err := New(logr.Discard(), nil); err == nil {
		t.Fatal("expected error when store is nil")
	}
}

type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return f.err
}

func TestReconcileReturnsGetError(t *testing.T) {
	store := NewModuleConfigStore(DefaultState())
	rec, err := New(logr.Discard(), store)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	rec.client = &failingClient{err: fmt.Errorf("boom")}

	_, err = rec.Reconcile(newContext(), ctrl.Request{NamespacedName: types.NamespacedName{Name: moduleConfigName}})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestReconcileReturnsParseError(t *testing.T) {
	store := NewModuleConfigStore(DefaultState())
	scheme := newModuleConfigScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": moduleConfigName},
		"spec": map[string]any{
			"enabled": true,
			"settings": map[string]any{
				"managedNodes": map[string]any{
					"enabledByDefault": "oops",
				},
			},
		},
	}}
	obj.SetGroupVersionKind(ModuleConfigGVK)

	if err := client.Create(context.Background(), obj.DeepCopy()); err != nil {
		t.Fatalf("seed client: %v", err)
	}

	rec, err := New(logr.Discard(), store)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.client = client

	_, err = rec.Reconcile(newContext(), ctrl.Request{NamespacedName: types.NamespacedName{Name: moduleConfigName}})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestExtractStateSettingsDecodeError(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"settings": "invalid",
		},
	}}
	obj.SetGroupVersionKind(ModuleConfigGVK)

	rec, err := New(logr.Discard(), NewModuleConfigStore(DefaultState()))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = rec.extractState(obj)
	if err == nil || !strings.Contains(err.Error(), "read spec.settings") {
		t.Fatalf("expected settings decode error, got %v", err)
	}
}

func TestSetupWithManagerConfiguresBuilder(t *testing.T) {
	scheme := newModuleConfigScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgr := newFakeManager(client, scheme)
	builderStub := &fakeBuilder{}
	store := NewModuleConfigStore(DefaultState())
	rec, err := New(testr.New(t), store)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec.build = func(ctrl.Manager) controllerbuilder.Builder { return builderStub }

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager returned error: %v", err)
	}

	if rec.client != client {
		t.Fatal("expected manager client captured")
	}
	if builderStub.named != controllerName {
		t.Fatalf("unexpected controller name: %s", builderStub.named)
	}
	if _, ok := builderStub.forObj.(*unstructured.Unstructured); !ok {
		t.Fatalf("unexpected For object: %T", builderStub.forObj)
	}
	if builderStub.options.MaxConcurrentReconciles != 1 {
		t.Fatalf("unexpected concurrency: %d", builderStub.options.MaxConcurrentReconciles)
	}
	if builderStub.options.LogConstructor == nil {
		t.Fatal("expected LogConstructor configured")
	}
	if builderStub.complete != 1 {
		t.Fatalf("expected Complete to be called once, got %d", builderStub.complete)
	}
}

func TestReconcileStoresDefaultsWhenMissing(t *testing.T) {
	store := NewModuleConfigStore(DefaultState())
	scheme := newModuleConfigScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler, err := New(logr.Discard(), store)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	reconciler.client = client

	_, err = reconciler.Reconcile(newContext(), ctrl.Request{NamespacedName: types.NamespacedName{Name: moduleConfigName}})
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	got := store.Current()
	if got.Settings.ManagedNodes.LabelKey == "" {
		t.Fatalf("expected defaults to be applied")
	}
}

func TestReconcileUpdatesStore(t *testing.T) {
	store := NewModuleConfigStore(DefaultState())
	scheme := newModuleConfigScheme()

	obj := &unstructured.Unstructured{}
	obj.Object = map[string]any{
		"metadata": map[string]any{
			"name": moduleConfigName,
		},
		"spec": map[string]any{
			"enabled": true,
			"settings": map[string]any{
				"managedNodes": map[string]any{
					"labelKey": "gpu.deckhouse.io/custom",
				},
			},
		},
	}
	obj.Object["kind"] = ModuleConfigGVK.Kind
	obj.Object["apiVersion"] = ModuleConfigGVK.GroupVersion().String()
	obj.SetGroupVersionKind(ModuleConfigGVK)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	if err := client.Create(context.Background(), obj.DeepCopy()); err != nil {
		t.Fatalf("failed to seed fake client with moduleconfig: %v", err)
	}

	reconciler, err := New(logr.Discard(), store)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	state, err := reconciler.extractState(obj.DeepCopy())
	if err != nil {
		t.Fatalf("extractState returned error: %v", err)
	}
	if state.Settings.ManagedNodes.LabelKey != "gpu.deckhouse.io/custom" {
		t.Fatalf("expected extracted labelKey to be updated, got %s", state.Settings.ManagedNodes.LabelKey)
	}
	reconciler.client = client

	_, err = reconciler.Reconcile(newContext(), ctrl.Request{NamespacedName: types.NamespacedName{Name: moduleConfigName}})
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	got := store.Current()
	if got.Settings.ManagedNodes.LabelKey != "gpu.deckhouse.io/custom" {
		t.Fatalf("expected labelKey to be updated, got %s", got.Settings.ManagedNodes.LabelKey)
	}
	if !got.Enabled {
		t.Fatalf("expected enabled flag true")
	}
}
