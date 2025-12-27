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
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent"
)

const (
	logDebugVerbosityEnv   = "LOG_DEBUG_VERBOSITY"
	logLevelEnv            = "LOG_LEVEL"
	logOutputEnv           = "LOG_OUTPUT"
	healthProbeBindAddrEnv = "HEALTH_PROBE_BIND_ADDRESS"
)

func main() {
	var probeAddr string
	var nodeName string
	var resyncPeriod time.Duration
	var sysRoot string
	var osReleasePath string
	var pciIDsPaths string

	logLevel := os.Getenv(logLevelEnv)
	logOutput := os.Getenv(logOutputEnv)
	logDebugVerbosity := envIntOrDie(logDebugVerbosityEnv)

	flag.StringVar(&probeAddr, "health-probe-bind-address", envOr(healthProbeBindAddrEnv, ":8081"), "The address the probe endpoint binds to.")
	flag.StringVar(&nodeName, "node-name", "", "Node name (defaults to NODE_NAME env var).")
	flag.DurationVar(&resyncPeriod, "resync-period", 0, "Resync period for scanning PCI devices (0s disables periodic resync).")
	flag.StringVar(&sysRoot, "sysfs-path", "/host-sys", "Path to the host sysfs mount.")
	flag.StringVar(&osReleasePath, "os-release-path", "/host-etc/os-release", "Path to the host os-release file.")
	flag.StringVar(&pciIDsPaths, "pci-ids-paths", "/host-usr-share/hwdata/pci.ids,/host-usr-share/misc/pci.ids,/host-usr-share/pci.ids,/host-usr-share/pciids/pci.ids", "Comma-separated list of pci.ids paths.")
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level.")
	flag.StringVar(&logOutput, "log-output", logOutput, "Log output.")
	flag.IntVar(&logDebugVerbosity, "log-debug-verbosity", logDebugVerbosity, "Log debug verbosity.")
	flag.Parse()

	rootLog := logger.NewLogger(logLevel, logOutput, logDebugVerbosity)
	logger.SetDefaultLogger(rootLog)
	log := rootLog.With(logger.SlogController("gpu-node-agent"))

	if nodeName == "" {
		nodeName = os.Getenv("NODE_NAME")
	}
	if nodeName == "" {
		log.Error("node name is required")
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gpuv1alpha1.AddToScheme(scheme))

	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		log.Error("unable to create Kubernetes client", logger.SlogErr(err))
		os.Exit(1)
	}

	agent := nodeagent.New(k8sClient, nodeagent.Config{
		NodeName:      nodeName,
		SysRoot:       sysRoot,
		OSReleasePath: osReleasePath,
		PCIIDsPaths:   splitComma(pciIDsPaths),
		ResyncPeriod:  resyncPeriod,
	}, log)

	ctx := ctrl.SetupSignalHandler()
	server := &http.Server{Addr: probeAddr, Handler: healthMux()}

	agentErrCh := make(chan error, 1)
	go func() {
		if err := agent.Run(ctx); err != nil {
			agentErrCh <- err
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting health server", "addr", probeAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-agentErrCh:
		log.Error("node agent failed", logger.SlogErr(err))
		os.Exit(1)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("health server failed", logger.SlogErr(err))
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("health server shutdown failed", logger.SlogErr(err))
		os.Exit(1)
	}
}

func healthMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
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

func splitComma(value string) []string {
	if value == "" {
		return nil
	}
	raw := strings.Split(value, ",")
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		paths = append(paths, item)
	}
	return paths
}
