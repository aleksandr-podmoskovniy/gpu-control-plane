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
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type FakeCache struct{ cache.Cache }

type StubFieldIndexer struct {
	client.FieldIndexer
	Err   error
	Calls int
}

func (s *StubFieldIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractor client.IndexerFunc) error {
	s.Calls++
	if s.Err != nil {
		return s.Err
	}
	return nil
}

type StubWebhookServer struct {
	Mux        *http.ServeMux
	Registered []string
}

func NewStubWebhookServer() *StubWebhookServer {
	return &StubWebhookServer{Mux: http.NewServeMux()}
}

func (s *StubWebhookServer) NeedLeaderElection() bool { return false }

func (s *StubWebhookServer) Register(path string, hook http.Handler) {
	s.Registered = append(s.Registered, path)
	if s.Mux != nil {
		s.Mux.Handle(path, hook)
	}
}

func (s *StubWebhookServer) Start(context.Context) error { return nil }

func (s *StubWebhookServer) StartedChecker() healthz.Checker {
	return func(*http.Request) error { return nil }
}

func (s *StubWebhookServer) WebhookMux() *http.ServeMux { return s.Mux }

var _ webhook.Server = (*StubWebhookServer)(nil)

type StubManager struct {
	manager.Manager

	Cache          cache.Cache
	Client         client.Client
	Scheme         *runtime.Scheme
	FieldIndexer   client.FieldIndexer
	Recorder       record.EventRecorder
	WebhookServer  webhook.Server
	AddErr         error
	Log            logr.Logger
	ControllerOpts ctrlconfig.Controller
	RestConfig     *rest.Config
}

func (m *StubManager) GetCache() cache.Cache { return m.Cache }

func (m *StubManager) GetClient() client.Client { return m.Client }

func (m *StubManager) GetScheme() *runtime.Scheme { return m.Scheme }

func (m *StubManager) GetFieldIndexer() client.FieldIndexer { return m.FieldIndexer }

func (m *StubManager) GetEventRecorderFor(string) record.EventRecorder { return m.Recorder }

func (m *StubManager) GetWebhookServer() webhook.Server { return m.WebhookServer }

func (m *StubManager) Add(manager.Runnable) error { return m.AddErr }

func (m *StubManager) GetLogger() logr.Logger {
	if m.Log.GetSink() != nil {
		return m.Log
	}
	return logr.Discard()
}

func (m *StubManager) GetControllerOptions() ctrlconfig.Controller { return m.ControllerOpts }

func (m *StubManager) GetConfig() *rest.Config { return m.RestConfig }

var _ manager.Manager = (*StubManager)(nil)

