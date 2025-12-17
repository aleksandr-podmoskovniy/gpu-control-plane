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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/clustergpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/gpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/poolusage"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

var (
	// Log is the base logger for the controller manager.
	Log = ctrl.Log.WithName("gpu-control-plane")

	newManager = ctrl.NewManager
	setupControllers = func(ctx context.Context, mgr ctrl.Manager, cfg config.ControllersConfig, store *moduleconfig.ModuleConfigStore) error {
		if err := moduleconfig.SetupController(ctx, mgr, Log, store); err != nil {
			return err
		}
		if err := inventory.SetupController(ctx, mgr, Log, cfg.GPUInventory, store); err != nil {
			return err
		}
		if err := bootstrap.SetupController(ctx, mgr, Log, cfg.GPUBootstrap, store); err != nil {
			return err
		}
		if err := gpupool.SetupController(ctx, mgr, Log, cfg.GPUPool, store); err != nil {
			return err
		}
		if err := clustergpupool.SetupController(ctx, mgr, Log, cfg.GPUPool, store); err != nil {
			return err
		}
		if err := poolusage.SetupGPUPoolUsageController(ctx, mgr, Log, cfg.GPUPool, store); err != nil {
			return err
		}
		if err := poolusage.SetupClusterGPUPoolUsageController(ctx, mgr, Log, cfg.GPUPool, store); err != nil {
			return err
		}
		return nil
	}
	getConfigOrDie = ctrl.GetConfigOrDie
	addGPUScheme   = v1alpha1.AddToScheme
	addNFDScheme   = nfdv1alpha1.AddToScheme
)

// Run initialises controller-runtime manager using the provided configuration and starts all controllers.
func Run(ctx context.Context, restCfg *rest.Config, sysCfg config.System) error {
	if restCfg == nil {
		restCfg = getConfigOrDie()
	}

	cpmetrics.Register()

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

	podReq, err := labels.NewRequirement(podlabels.PoolNameKey, selection.Exists, nil)
	if err != nil {
		return fmt.Errorf("build pod cache label selector: %w", err)
	}
	gpuPodSelector := labels.NewSelector().Add(*podReq)

	options := manager.Options{
		Metrics:                metricsOpts,
		HealthProbeBindAddress: ":8081",
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Pod{}: {
					Namespaces: map[string]cache.Config{
						// Control-plane workloads (validator, GFD, etc.) live here and must be fully cached.
						meta.WorkloadsNamespace: {},
						// Workload pods across the cluster are cached only when they request GPU resources
						// and were labeled by the mutating webhook.
						cache.AllNamespaces: {LabelSelector: gpuPodSelector},
					},
				},
			},
		},
		WebhookServer: crwebhook.NewServer(crwebhook.Options{
			Port:    9443,
			CertDir: "/var/lib/gpu-control-plane/tls",
		}),
	}

	if sysCfg.LeaderElection.Enabled {
		options.LeaderElection = true
		options.LeaderElectionNamespace = sysCfg.LeaderElection.Namespace
		options.LeaderElectionID = sysCfg.LeaderElection.ID
		options.LeaderElectionResourceLock = sysCfg.LeaderElection.ResourceLock
		options.LeaderElectionReleaseOnCancel = true
	}

	mgr, err := newManager(restCfg, options)
	if err != nil {
		return fmt.Errorf("new manager: %w", err)
	}

	if err := addGPUScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("register gpu scheme: %w", err)
	}
	nfdScheme := mgr.GetScheme()
	if err := addNFDScheme(nfdScheme); err != nil {
		return fmt.Errorf("register nfd scheme: %w", err)
	}
	// Register list types explicitly because upstream AddToScheme omits them.
	nfdScheme.AddKnownTypes(
		nfdv1alpha1.SchemeGroupVersion,
		&nfdv1alpha1.NodeFeatureList{},
		&nfdv1alpha1.NodeFeatureRuleList{},
		&nfdv1alpha1.NodeFeatureGroupList{},
	)

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("healthz: %w", err)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("readyz: %w", err)
	}

	moduleState, err := config.ModuleSettingsToState(sysCfg.Module)
	if err != nil {
		return fmt.Errorf("convert module settings: %w", err)
	}
	store := moduleconfig.NewModuleConfigStore(moduleState)

	if err := setupControllers(ctx, mgr, sysCfg.Controllers, store); err != nil {
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

	if addr := strings.TrimSpace(os.Getenv("METRICS_BIND_ADDRESS")); addr != "" {
		opts.BindAddress = addr
	}

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
