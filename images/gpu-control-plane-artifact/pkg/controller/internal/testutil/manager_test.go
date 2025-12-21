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

package testutil

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
)

func TestStubFieldIndexerIndexField(t *testing.T) {
	calls := 0
	s := &StubFieldIndexer{FieldIndexer: nil}

	if err := s.IndexField(context.Background(), nil, "field", func(client.Object) []string { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	calls++
	if s.Calls != calls {
		t.Fatalf("expected %d calls, got %d", calls, s.Calls)
	}

	expected := errors.New("index fail")
	s.Err = expected
	if err := s.IndexField(context.Background(), nil, "field", func(client.Object) []string { return nil }); !errors.Is(err, expected) {
		t.Fatalf("expected error, got %v", err)
	}
	calls++
	if s.Calls != calls {
		t.Fatalf("expected %d calls, got %d", calls, s.Calls)
	}
}

func TestStubWebhookServer(t *testing.T) {
	s := NewStubWebhookServer()
	if s.WebhookMux() == nil {
		t.Fatalf("expected mux to be initialized")
	}
	if s.NeedLeaderElection() {
		t.Fatalf("expected NeedLeaderElection to be false")
	}

	s.Register("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if len(s.Registered) != 1 || s.Registered[0] != "/test" {
		t.Fatalf("unexpected registered paths: %v", s.Registered)
	}

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	_, pattern := s.WebhookMux().Handler(req)
	if pattern != "/test" {
		t.Fatalf("expected handler pattern to be /test, got %q", pattern)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if err := s.StartedChecker()(req); err != nil {
		t.Fatalf("unexpected started checker error: %v", err)
	}
}

func TestStubManagerAccessors(t *testing.T) {
	scheme := runtime.NewScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := record.NewFakeRecorder(8)
	opts := ctrlconfig.Controller{GroupKindConcurrency: map[string]int{"a": 1}}
	restCfg := &rest.Config{Host: "example"}

	m := &StubManager{
		Cache:          &FakeCache{},
		Client:         cl,
		Scheme:         scheme,
		FieldIndexer:   &StubFieldIndexer{},
		Recorder:       rec,
		WebhookServer:  NewStubWebhookServer(),
		AddErr:         errors.New("add fail"),
		Log:            testr.New(t),
		ControllerOpts: opts,
		RestConfig:     restCfg,
	}

	if m.GetCache() != m.Cache {
		t.Fatalf("unexpected cache")
	}
	if m.GetClient() != m.Client {
		t.Fatalf("unexpected client")
	}
	if m.GetScheme() != m.Scheme {
		t.Fatalf("unexpected scheme")
	}
	if m.GetFieldIndexer() != m.FieldIndexer {
		t.Fatalf("unexpected field indexer")
	}
	if m.GetEventRecorderFor("x") != m.Recorder {
		t.Fatalf("unexpected recorder")
	}
	if m.GetWebhookServer() != m.WebhookServer {
		t.Fatalf("unexpected webhook server")
	}
	if err := m.Add(nil); !errors.Is(err, m.AddErr) {
		t.Fatalf("expected add error, got %v", err)
	}
	if m.GetControllerOptions().GroupKindConcurrency["a"] != opts.GroupKindConcurrency["a"] {
		t.Fatalf("unexpected controller options")
	}
	if m.GetConfig() != restCfg {
		t.Fatalf("unexpected rest config")
	}

	assertNotPanics := func(t *testing.T, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}()
		fn()
	}

	assertNotPanics(t, func() { m.GetLogger().Info("logger") })

	m.Log = logr.Logger{}
	assertNotPanics(t, func() { m.GetLogger().Info("fallback") })
}
