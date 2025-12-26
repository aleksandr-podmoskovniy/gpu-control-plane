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
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const (
	logDebugVerbosityEnv   = "LOG_DEBUG_VERBOSITY"
	logLevelEnv            = "LOG_LEVEL"
	logOutputEnv           = "LOG_OUTPUT"
	healthProbeBindAddrEnv = "HEALTH_PROBE_BIND_ADDRESS"
	metricsBindAddrEnv     = "METRICS_BIND_ADDRESS"
)

func main() {
	var probeAddr string
	var metricsAddr string

	logLevel := os.Getenv(logLevelEnv)
	logOutput := os.Getenv(logOutputEnv)
	logDebugVerbosity := envIntOrDie(logDebugVerbosityEnv)

	flag.StringVar(&probeAddr, "health-probe-bind-address", envOr(healthProbeBindAddrEnv, ":8081"), "The address the probe endpoint binds to.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", envOr(metricsBindAddrEnv, "127.0.0.1:8080"), "The address the metrics endpoint binds to.")
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level.")
	flag.StringVar(&logOutput, "log-output", logOutput, "Log output.")
	flag.IntVar(&logDebugVerbosity, "log-debug-verbosity", logDebugVerbosity, "Log debug verbosity.")
	flag.Parse()

	rootLog := logger.NewLogger(logLevel, logOutput, logDebugVerbosity)
	logger.SetDefaultLogger(rootLog)
	log := rootLog.With(logger.SlogController("gpu-dra-controller"))

	ctx := ctrl.SetupSignalHandler()
	server := &http.Server{Addr: probeAddr, Handler: healthMux()}
	metricsServer := &http.Server{Addr: metricsAddr, Handler: promhttp.Handler()}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting health server", "addr", probeAddr)
		errCh <- server.ListenAndServe()
	}()

	if metricsAddr != "" && metricsAddr != "0" {
		go func() {
			log.Info("starting metrics server", "addr", metricsAddr)
			errCh <- metricsServer.ListenAndServe()
		}()
	}

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
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
	if metricsAddr != "" && metricsAddr != "0" {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error("metrics server shutdown failed", logger.SlogErr(err))
			os.Exit(1)
		}
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
