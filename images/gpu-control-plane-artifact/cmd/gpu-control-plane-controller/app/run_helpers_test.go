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

package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

func generateSelfSignedCert(t *testing.T) ([]byte, []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "example.local"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return certPEM, keyPEM
}

type fakeManager struct {
	config      *rest.Config
	scheme      *runtime.Scheme
	startCalled bool
	startErr    error
	healthErr   error
	readyErr    error
}

func newFakeManager() *fakeManager {
	return &fakeManager{
		scheme: runtime.NewScheme(),
	}
}

// cluster.Cluster methods.
func (f *fakeManager) GetHTTPClient() *http.Client                     { return nil }
func (f *fakeManager) GetConfig() *rest.Config                         { return f.config }
func (f *fakeManager) GetCache() cache.Cache                           { return nil }
func (f *fakeManager) GetScheme() *runtime.Scheme                      { return f.scheme }
func (f *fakeManager) GetClient() client.Client                        { return nil }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer            { return nil }
func (f *fakeManager) GetEventRecorderFor(string) record.EventRecorder { return nil }
func (f *fakeManager) GetRESTMapper() meta.RESTMapper                  { return nil }
func (f *fakeManager) GetAPIReader() client.Reader                     { return nil }
func (f *fakeManager) Start(context.Context) error {
	f.startCalled = true
	return f.startErr
}

// manager.Manager methods.
func (f *fakeManager) Add(ctrlmanager.Runnable) error { return nil }

func (f *fakeManager) Elected() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (f *fakeManager) AddMetricsServerExtraHandler(string, http.Handler) error { return nil }

func (f *fakeManager) AddHealthzCheck(string, healthz.Checker) error {
	if f.healthErr != nil {
		return f.healthErr
	}
	return nil
}

func (f *fakeManager) AddReadyzCheck(string, healthz.Checker) error {
	if f.readyErr != nil {
		return f.readyErr
	}
	return nil
}

func (f *fakeManager) GetWebhookServer() ctrlwebhook.Server { return nil }

func (f *fakeManager) GetLogger() logr.Logger { return ctrl.Log }

func (f *fakeManager) GetControllerOptions() crconfig.Controller { return crconfig.Controller{} }

var _ ctrlmanager.Manager = (*fakeManager)(nil)
