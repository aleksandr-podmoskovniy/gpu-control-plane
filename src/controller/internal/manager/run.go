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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/pkg/apis/nfd/v1alpha1"
)

var (
	// Log is the base logger for the controller manager.
	Log = ctrl.Log.WithName("gpu-control-plane")
)

// Run initialises controller-runtime manager using the provided configuration and starts all controllers.
func Run(ctx context.Context, restCfg *rest.Config, sysCfg config.System) error {
	if restCfg == nil {
		restCfg = ctrl.GetConfigOrDie()
	}

	metricsOpts, err := metricsOptionsFromEnv()
	if err != nil {
		Log.Error(err, "using insecure metrics endpoint due to TLS configuration error")
		metricsOpts = server.Options{BindAddress: server.DefaultBindAddress}
	} else if metricsOpts.SecureServing {
		Log.Info("metrics server configured for TLS",
			"certDir", metricsOpts.CertDir,
			"cert", metricsOpts.CertName,
			"key", metricsOpts.KeyName)
	}

	options := manager.Options{
		Metrics:                metricsOpts,
		HealthProbeBindAddress: ":8081",
	}

	if sysCfg.LeaderElection.Enabled {
		options.LeaderElection = true
		options.LeaderElectionNamespace = sysCfg.LeaderElection.Namespace
		options.LeaderElectionID = sysCfg.LeaderElection.ID
		options.LeaderElectionResourceLock = sysCfg.LeaderElection.ResourceLock
		options.LeaderElectionReleaseOnCancel = true
	}

	mgr, err := ctrl.NewManager(restCfg, options)
	if err != nil {
		return fmt.Errorf("new manager: %w", err)
	}

	if err := v1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("register gpu scheme: %w", err)
	}
	if err := nfdv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("register nfd scheme: %w", err)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("healthz: %w", err)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("readyz: %w", err)
	}

	deps := controllers.Dependencies{
		Logger:            Log,
		InventoryHandlers: contracts.NewInventoryRegistry(),
		BootstrapHandlers: contracts.NewBootstrapRegistry(),
		PoolHandlers:      contracts.NewPoolRegistry(),
		AdmissionHandlers: contracts.NewAdmissionRegistry(),
	}

	handlers.RegisterDefaults(Log, &handlers.Handlers{
		Inventory: deps.InventoryHandlers,
		Bootstrap: deps.BootstrapHandlers,
		Pool:      deps.PoolHandlers,
		Admission: deps.AdmissionHandlers,
	})

	if err := controllers.Register(ctx, mgr, sysCfg.Controllers, sysCfg.Module, deps); err != nil {
		return fmt.Errorf("register controllers: %w", err)
	}

	Log.Info("starting manager", "goVersion", runtime.Version())
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager start: %w", err)
	}
	return nil
}

func metricsOptionsFromEnv() (server.Options, error) {
	opts := server.Options{BindAddress: server.DefaultBindAddress}

	certFile := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("TLS_PRIVATE_KEY_FILE"))

	if certFile == "" || keyFile == "" {
		return opts, nil
	}

	if _, err := os.Stat(certFile); err != nil {
		return opts, fmt.Errorf("stat TLS_CERT_FILE: %w", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return opts, fmt.Errorf("stat TLS_PRIVATE_KEY_FILE: %w", err)
	}

	opts.SecureServing = true
	opts.CertDir = filepath.Dir(certFile)
	opts.CertName = filepath.Base(certFile)
	opts.KeyName = filepath.Base(keyFile)

	if caFile := strings.TrimSpace(os.Getenv("TLS_CA_FILE")); caFile != "" {
		caData, err := os.ReadFile(caFile)
		if err != nil {
			return opts, fmt.Errorf("read TLS_CA_FILE: %w", err)
		}

		caPool := x509.NewCertPool()
		if ok := caPool.AppendCertsFromPEM(caData); !ok {
			return opts, fmt.Errorf("parse TLS_CA_FILE: no certificates found")
		}

		opts.TLSOpts = append(opts.TLSOpts, func(cfg *tls.Config) {
			cfg.ClientCAs = caPool
			cfg.ClientAuth = tls.VerifyClientCertIfGiven
		})
	}

	return opts, nil
}
