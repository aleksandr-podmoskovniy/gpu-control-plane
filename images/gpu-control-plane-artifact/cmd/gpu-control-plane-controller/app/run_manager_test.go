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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestRunSuccess(t *testing.T) {
	origNewManager := newManager
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
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

	controllersCalled := false
	var receivedCtx context.Context
	setupControllers = func(ctx context.Context, mgr ctrlmanager.Manager, cfg config.ControllersConfig, store *moduleconfig.ModuleConfigStore) error {
		controllersCalled = true
		receivedCtx = ctx
		if mgr != fakeMgr {
			t.Fatalf("expected fake manager")
		}
		if store == nil {
			t.Fatalf("module config store must be provided")
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

	if !controllersCalled {
		t.Fatalf("setupControllers was not called")
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	var capturedOptions ctrlmanager.Options
	newManager = func(cfg *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		capturedOptions = opts
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
	})

	newManager = func(cfg *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return newFakeManager(), nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
		t.Fatalf("setupControllers must not be called when module settings are invalid")
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
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
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
		return errors.New("controllers failed")
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "register controllers: controllers failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunManagerStartError(t *testing.T) {
	origNewManager := newManager
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie
	origAddGPUScheme := addGPUScheme
	origAddNFDScheme := addNFDScheme

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
		addGPUScheme = origAddGPUScheme
		addNFDScheme = origAddNFDScheme
	})

	fakeMgr := newFakeManager()
	fakeMgr.startErr = errors.New("run failed")
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie
	origAddGPUScheme := addGPUScheme
	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
		addGPUScheme = origAddGPUScheme
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie
	origAddGPUScheme := addGPUScheme
	origAddNFDScheme := addNFDScheme
	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
		addGPUScheme = origAddGPUScheme
		addNFDScheme = origAddNFDScheme
	})

	fakeMgr := newFakeManager()
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie
	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	fakeMgr.healthErr = errors.New("health failed")
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
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
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie
	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
	})

	fakeMgr := newFakeManager()
	fakeMgr.readyErr = errors.New("ready failed")
	newManager = func(*rest.Config, ctrlmanager.Options) (ctrlmanager.Manager, error) {
		return fakeMgr, nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config { return &rest.Config{} }

	err := Run(context.Background(), nil, config.DefaultSystem())
	if err == nil || err.Error() != "readyz: ready failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}
