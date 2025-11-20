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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/pkg/detect"
)

var defaultShutdownTimeout = 5 * time.Second

// Detector exposes GPU telemetry collected from NVML.
type Detector interface {
	DetectGPU(ctx context.Context) ([]detect.Info, error)
}

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

// Config controls the HTTP server behaviour.
type Config struct {
	ListenAddr      string
	Path            string
	ShutdownTimeout time.Duration
}

// Server exposes a read-only API for gpu-control-plane controller.
type Server struct {
	cfg       Config
	detector  Detector
	logger    *slog.Logger
	httpSrv   httpServer
	factory   func(addr string, handler http.Handler) httpServer
	startedCh chan struct{}
	registry  *prometheus.Registry
}

// New constructs a Server.
func New(cfg Config, detector Detector, logger *slog.Logger) *Server {
	timeout := cfg.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(detectRequests, detectWarnings, detectDuration)

	return &Server{
		cfg: Config{
			ListenAddr:      cfg.ListenAddr,
			Path:            cfg.Path,
			ShutdownTimeout: timeout,
		},
		detector: detector,
		logger:   logger,
		factory: func(addr string, handler http.Handler) httpServer {
			return &stdHTTPServer{srv: &http.Server{
				Addr:    addr,
				Handler: handler,
			}}
		},
		startedCh: make(chan struct{}),
		registry:  registry,
	}
}

// Run blocks until the context is cancelled or the HTTP server fails.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Path, s.handleDetect)
	mux.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	s.httpSrv = s.factory(s.cfg.ListenAddr, mux)

	errCh := make(chan error, 1)
	go func() {
		close(s.startedCh)
		s.logger.Info("gfd-extender server started",
			slog.String("addr", s.cfg.ListenAddr),
			slog.String("path", s.cfg.Path),
		)
		errCh <- s.httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		detectRequests.WithLabelValues("method_not_allowed").Inc()
		return
	}

	ctx := r.Context()
	start := time.Now()
	infos, err := s.detector.DetectGPU(ctx)
	if err != nil {
		s.logger.Error("detect request failed",
			slog.String("remote", r.RemoteAddr),
			slog.String("error", err.Error()),
		)
		http.Error(w, "failed to collect GPU data", http.StatusInternalServerError)
		detectRequests.WithLabelValues("error").Inc()
		return
	}

	warnCount := 0
	for _, info := range infos {
		for _, warn := range info.Warnings {
			warnCount++
			s.logger.Warn("partial GPU data from gfd-extender",
				slog.Int("index", info.Index),
				slog.String("uuid", info.UUID),
				slog.String("warning", warn),
			)
		}
	}
	if warnCount > 0 {
		detectWarnings.Add(float64(warnCount))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(infos); err != nil {
		s.logger.Error("failed to encode response",
			slog.String("error", err.Error()),
		)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		detectRequests.WithLabelValues("error").Inc()
		return
	}

	s.logger.Info("detect request served",
		slog.Int("devices", len(infos)),
		slog.Int("warnings", warnCount),
		slog.Duration("duration", time.Since(start)),
	)
	detectRequests.WithLabelValues("ok").Inc()
	detectDuration.Observe(time.Since(start).Seconds())
}

func (s *Server) shutdown() error {
	if s.httpSrv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	err := s.httpSrv.Shutdown(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	s.logger.Info("gfd-extender server stopped")
	return nil
}

type stdHTTPServer struct {
	srv *http.Server
}

func (s *stdHTTPServer) ListenAndServe() error {
	return s.srv.ListenAndServe()
}

func (s *stdHTTPServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
