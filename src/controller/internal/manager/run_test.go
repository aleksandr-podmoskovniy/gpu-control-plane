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

package manager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/webhook"
)

func TestMetricsOptionsFromEnvDefault(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_PRIVATE_KEY_FILE", "")
	t.Setenv("TLS_CA_FILE", "")

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SecureServing {
		t.Fatalf("expected SecureServing to be disabled by default")
	}
}

func TestMetricsOptionsFromEnvTLS(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	caPath := filepath.Join(dir, "ca.crt")

	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	t.Setenv("TLS_CA_FILE", caPath)

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.SecureServing {
		t.Fatalf("expected SecureServing to be enabled")
	}
	if opts.CertDir != dir {
		t.Fatalf("unexpected cert dir: %s", opts.CertDir)
	}
	if opts.CertName != "tls.crt" {
		t.Fatalf("unexpected cert name: %s", opts.CertName)
	}
	if opts.KeyName != "tls.key" {
		t.Fatalf("unexpected key name: %s", opts.KeyName)
	}
	if len(opts.TLSOpts) == 0 {
		t.Fatalf("expected TLSOpts to include CA configuration")
	}
	cfg := &tls.Config{}
	opts.TLSOpts[0](cfg)
	if cfg.ClientCAs == nil {
		t.Fatalf("expected ClientCAs to be configured")
	}
	if cfg.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Fatalf("unexpected client auth mode: %v", cfg.ClientAuth)
	}
}

func TestMetricsOptionsFromEnvInvalidCA(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	caPath := filepath.Join(dir, "ca.crt")

	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caPath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	t.Setenv("TLS_CA_FILE", caPath)

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error for invalid CA data")
	}
}

func TestMetricsOptionsFromEnvMissingCert(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", filepath.Join(t.TempDir(), "absent.crt"))
	t.Setenv("TLS_PRIVATE_KEY_FILE", filepath.Join(t.TempDir(), "absent.key"))

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error when TLS files are missing")
	}
}

func TestMetricsOptionsFromEnvMissingKey(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	// Ensure key file exists separately to avoid short-circuit on missing cert.
	keyPath := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", filepath.Join(dir, "missing.key"))

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error when private key is missing")
	}
}

func TestMetricsOptionsFromEnvReadCAError(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	certPEM, keyPEM := generateSelfSignedCert(t)

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	// Point to a missing CA file to trigger read error.
	t.Setenv("TLS_CA_FILE", filepath.Join(dir, "missing-ca.crt"))

	if _, err := metricsOptionsFromEnv(); err == nil {
		t.Fatalf("expected error when CA file cannot be read")
	}
}

func TestMetricsOptionsFromEnvCustomBindAddress(t *testing.T) {
	t.Setenv("METRICS_BIND_ADDRESS", "  :9444  ")
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_PRIVATE_KEY_FILE", "")
	t.Setenv("TLS_CA_FILE", "")

	opts, err := metricsOptionsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.BindAddress != ":9444" {
		t.Fatalf("expected bind address :9444, got %s", opts.BindAddress)
	}
}

func TestRunSuccess(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	var capturedOptions ctrlmanager.Options
	newManager = func(cfg *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		if cfg == nil {
			t.Fatalf("rest config should not be nil")
		}
		capturedOptions = opts
		fakeMgr.config = cfg
		return fakeMgr, nil
	}

	handlersCalled := false
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		handlersCalled = true
		origRegisterHandlers(log, deps)
		if deps.Inventory == nil || deps.Bootstrap == nil || deps.Pool == nil || deps.Admission == nil {
			t.Fatalf("expected registries to be initialised")
		}
	}

	controllersCalled := false
	var receivedCtx context.Context
	registerControllers = func(ctx context.Context, mgr ctrlmanager.Manager, cfg config.ControllersConfig, _ *config.ModuleConfigStore, deps controllers.Dependencies) error {
		controllersCalled = true
		receivedCtx = ctx
		if mgr != fakeMgr {
			t.Fatalf("expected fake manager")
		}
		if len(deps.InventoryHandlers.List()) == 0 {
			t.Fatalf("inventory registry should be populated")
		}
		return nil
	}

	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	sysCfg := config.DefaultSystem()
	sysCfg.LeaderElection.Enabled = true
	sysCfg.LeaderElection.Namespace = "gpu-ops"
	sysCfg.LeaderElection.ID = "gpu-manager"
	sysCfg.LeaderElection.ResourceLock = "leases"

	ctx := context.Background()
	if err := Run(ctx, nil, sysCfg); err != nil {
		t.Fatalf("unexpected Run error: %v", err)
	}

	if !handlersCalled {
		t.Fatalf("registerHandlers was not called")
	}
	if !controllersCalled {
		t.Fatalf("registerControllers was not called")
	}
	if !fakeMgr.startCalled {
		t.Fatalf("manager Start must be invoked")
	}
	if receivedCtx != ctx {
		t.Fatalf("expected context propagation to controllers")
	}

	if !capturedOptions.LeaderElection {
		t.Fatalf("leader election must be enabled")
	}
	if capturedOptions.LeaderElectionNamespace != "gpu-ops" {
		t.Fatalf("unexpected leader election namespace: %s", capturedOptions.LeaderElectionNamespace)
	}
	if capturedOptions.LeaderElectionID != "gpu-manager" {
		t.Fatalf("unexpected leader election id: %s", capturedOptions.LeaderElectionID)
	}
	if capturedOptions.LeaderElectionResourceLock != "leases" {
		t.Fatalf("unexpected resource lock: %s", capturedOptions.LeaderElectionResourceLock)
	}
	if capturedOptions.HealthProbeBindAddress != ":8081" {
		t.Fatalf("unexpected health probe address: %s", capturedOptions.HealthProbeBindAddress)
	}
}

func TestRunMetricsTLSFallbackOnError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	var capturedOptions ctrlmanager.Options
	newManager = func(cfg *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		capturedOptions = opts
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	t.Setenv("TLS_CERT_FILE", filepath.Join(t.TempDir(), "missing.crt"))
	t.Setenv("TLS_PRIVATE_KEY_FILE", filepath.Join(t.TempDir(), "missing.key"))

	if err := Run(context.Background(), nil, config.DefaultSystem()); err != nil {
		t.Fatalf("unexpected Run error: %v", err)
	}

	if capturedOptions.Metrics.SecureServing {
		t.Fatalf("secure metrics must be disabled when TLS setup fails")
	}
}

func TestRunFailsOnInvalidModuleSettings(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	newManager = func(cfg *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return newFakeManager(), nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		t.Fatalf("registerControllers must not be called when module settings are invalid")
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	sysCfg := config.DefaultSystem()
	sysCfg.Module.Scheduling.DefaultStrategy = "invalid-strategy"

	err := Run(context.Background(), nil, sysCfg)
	if err == nil || !strings.Contains(err.Error(), "convert module settings") {
		t.Fatalf("expected module settings conversion error, got %v", err)
	}
}

func TestRunWithSecureMetrics(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	certPEM, keyPEM := generateSelfSignedCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv("TLS_CERT_FILE", certPath)
	t.Setenv("TLS_PRIVATE_KEY_FILE", keyPath)
	t.Setenv("TLS_CA_FILE", "")

	fakeMgr := newFakeManager()
	var capturedOptions ctrlmanager.Options
	newManager = func(cfg *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		capturedOptions = opts
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	if err := Run(context.Background(), nil, config.DefaultSystem()); err != nil {
		t.Fatalf("unexpected Run error: %v", err)
	}

	if !capturedOptions.Metrics.SecureServing {
		t.Fatalf("secure metrics must be enabled when TLS files exist")
	}
	if capturedOptions.Metrics.CertDir != dir {
		t.Fatalf("unexpected metrics cert dir: %s", capturedOptions.Metrics.CertDir)
	}
	if capturedOptions.Metrics.CertName != "tls.crt" {
		t.Fatalf("unexpected metrics cert name: %s", capturedOptions.Metrics.CertName)
	}
	if capturedOptions.Metrics.KeyName != "tls.key" {
		t.Fatalf("unexpected metrics key name: %s", capturedOptions.Metrics.KeyName)
	}
}

func TestRunNewManagerError(t *testing.T) {
	origNewManager := newManager
	origGetConfig := getConfigOrDie
	t.Cleanup(func() {
		newManager = origNewManager
		getConfigOrDie = origGetConfig
	})

	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return nil, errors.New("boom")
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "new manager: boom" {
		t.Fatalf("expected wrapped manager creation error, got %v", err)
	}
}

func TestRunRegisterControllersError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return errors.New("controllers failed")
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "register controllers: controllers failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRegisterWebhooksError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	registerWebhooks = func(context.Context, ctrlmanager.Manager, webhook.Dependencies) error {
		return errors.New("webhook fail")
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "register webhooks: webhook fail" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunManagerStartError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie
	origAddGPUScheme := addGPUScheme
	origAddNFDScheme := addNFDScheme

	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
		addGPUScheme = origAddGPUScheme
		addNFDScheme = origAddNFDScheme
	})

	fakeMgr := newFakeManager()
	fakeMgr.startErr = errors.New("run failed")
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "manager start: run failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRegisterGPUSchemeError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie
	origAddGPUScheme := addGPUScheme
	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
		addGPUScheme = origAddGPUScheme
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }
	addGPUScheme = func(*runtime.Scheme) error { return errors.New("gpu scheme failure") }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "register gpu scheme: gpu scheme failure" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRegisterNFDSchemeError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie
	origAddGPUScheme := addGPUScheme
	origAddNFDScheme := addNFDScheme
	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
		addGPUScheme = origAddGPUScheme
		addNFDScheme = origAddNFDScheme
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }
	addNFDScheme = func(*runtime.Scheme) error { return errors.New("nfd scheme failure") }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "register nfd scheme: nfd scheme failure" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHealthzCheckError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie
	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	fakeMgr.healthErr = errors.New("health failed")
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "healthz: health failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReadyzCheckError(t *testing.T) {
	origNewManager := newManager
	origRegisterHandlers := registerHandlers
	origRegisterControllers := registerControllers
	origRegisterWebhooks := registerWebhooks
	origGetConfig := getConfigOrDie
	t.Cleanup(func() {
		newManager = origNewManager
		registerHandlers = origRegisterHandlers
		registerControllers = origRegisterControllers
		registerWebhooks = origRegisterWebhooks
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	fakeMgr.readyErr = errors.New("ready failed")
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	registerHandlers = func(log logr.Logger, deps *handlers.Handlers) {
		origRegisterHandlers(log, deps)
	}
	registerControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *config.ModuleConfigStore, controllers.Dependencies) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "readyz: ready failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
