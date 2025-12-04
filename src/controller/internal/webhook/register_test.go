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

package webhook

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr/testr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type stubServer struct {
	registered map[string]http.Handler
}

func (s *stubServer) NeedLeaderElection() bool { return false }
func (s *stubServer) Start(context.Context) error {
	return nil
}
func (s *stubServer) StartedChecker() healthz.Checker {
	return func(_ *http.Request) error { return nil }
}
func (s *stubServer) WebhookMux() *http.ServeMux { return http.NewServeMux() }
func (s *stubServer) Register(path string, hook http.Handler) {
	if s.registered == nil {
		s.registered = map[string]http.Handler{}
	}
	s.registered[path] = hook
}

type stubManager struct {
	server ctrlwebhook.Server
	scheme *runtime.Scheme
}

func (m *stubManager) GetWebhookServer() ctrlwebhook.Server { return m.server }
func (m *stubManager) GetScheme() *runtime.Scheme           { return m.scheme }

func TestRegisterSkipsWhenNoServer(t *testing.T) {
	mgr := &stubManager{server: nil, scheme: runtime.NewScheme()}
	if err := Register(context.Background(), mgr, Dependencies{}); err != nil {
		t.Fatalf("expected nil error when no server, got %v", err)
	}
}

func TestRegisterHooks(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	server := &stubServer{}
	mgr := &stubManager{server: server, scheme: scheme}

	reg := contracts.NewAdmissionRegistry()
	reg.Register(contracts.AdmissionHandlerFunc(func(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
		return contracts.Result{}, nil
	}))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := Register(context.Background(), mgr, Dependencies{
		Logger:            testr.New(t),
		AdmissionHandlers: reg,
		Client:            client,
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	if len(server.registered) != 5 {
		t.Fatalf("expected 5 hooks registered, got %d", len(server.registered))
	}
	if _, ok := server.registered["/validate-gpu-deckhouse-io-v1alpha1-gpupool"]; !ok {
		t.Fatalf("validator not registered")
	}
	if _, ok := server.registered["/validate-gpu-deckhouse-io-v1alpha1-gpudevice"]; !ok {
		t.Fatalf("device validator not registered")
	}
	if _, ok := server.registered["/mutate-gpu-deckhouse-io-v1alpha1-gpupool"]; !ok {
		t.Fatalf("defaulter not registered")
	}
	if _, ok := server.registered["/mutate-v1-pod-gpupool"]; !ok {
		t.Fatalf("pod mutator not registered")
	}
	if _, ok := server.registered["/validate-v1-pod-gpupool"]; !ok {
		t.Fatalf("pod validator not registered")
	}

	validator := server.registered["/validate-gpu-deckhouse-io-v1alpha1-gpupool"].(*ctrlwebhook.Admission)
	req := cradmission.Request{}
	if resp := validator.Handle(context.Background(), req); resp.Result == nil {
		t.Fatalf("expected non-nil response from validator")
	}
}
