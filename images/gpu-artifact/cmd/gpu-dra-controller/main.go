/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.).
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const (
	logDebugVerbosityEnv = "LOG_DEBUG_VERBOSITY"
	logLevelEnv          = "LOG_LEVEL"
	logOutputEnv         = "LOG_OUTPUT"

	metricsBindAddrEnv     = "METRICS_BIND_ADDRESS"
	healthProbeBindAddrEnv = "HEALTH_PROBE_BIND_ADDRESS"
	pprofBindAddrEnv       = "PPROF_BIND_ADDRESS"
	podNamespaceEnv        = "POD_NAMESPACE"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(resourcev1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var pprofAddr string
	var enableLeaderElection bool
	var leaderElectionID string

	logLevel := os.Getenv(logLevelEnv)
	logOutput := os.Getenv(logOutputEnv)
	logDebugVerbosity := envIntOrDie(logDebugVerbosityEnv)

	flag.StringVar(&metricsAddr, "metrics-bind-address", envOr(metricsBindAddrEnv, ":8080"), "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", envOr(healthProbeBindAddrEnv, ":8083"), "The address the probe endpoint binds to.")
	flag.StringVar(&pprofAddr, "pprof-bind-address", envOr(pprofBindAddrEnv, ""), "Enable pprof endpoint when set.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for the controller.")
	flag.StringVar(&leaderElectionID, "leader-election-id", "gpu-dra-controller.deckhouse.io", "Leader election ID.")
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level.")
	flag.StringVar(&logOutput, "log-output", logOutput, "Log output.")
	flag.IntVar(&logDebugVerbosity, "log-debug-verbosity", logDebugVerbosity, "Log debug verbosity.")
	flag.Parse()

	rootLog := logger.NewLogger(logLevel, logOutput, logDebugVerbosity)
	logger.SetDefaultLogger(rootLog)
	setupLog := rootLog.With(logger.SlogController("gpu-dra-controller"))

	leaderElectionNS := envOr(podNamespaceEnv, "default")
	managerOpts := ctrl.Options{
		Scheme:                        scheme,
		Metrics:                       metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress:        probeAddr,
		LeaderElection:                enableLeaderElection,
		LeaderElectionNamespace:       leaderElectionNS,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionReleaseOnCancel: true,
	}
	if pprofAddr != "" {
		managerOpts.PprofBindAddress = pprofAddr
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), managerOpts)
	if err != nil {
		setupLog.Error("unable to start manager", logger.SlogErr(err))
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	draLog := logger.NewControllerLogger(dra.ControllerName, logLevel, logOutput, logDebugVerbosity, nil)
	if err := dra.SetupController(ctx, mgr, draLog); err != nil {
		setupLog.Error("unable to create controller", "controller", dra.ControllerName, logger.SlogErr(err))
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error("unable to set up health check", logger.SlogErr(err))
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error("unable to set up ready check", logger.SlogErr(err))
		os.Exit(1)
	}

	setupLog.Info("starting gpu-dra-controller")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error("problem running manager", logger.SlogErr(err))
		os.Exit(1)
	}
}

func envOr(name, fallback string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	return fallback
}

func envIntOrDie(name string) int {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid %s: %v\n", name, err)
		os.Exit(1)
	}
	return val
}
