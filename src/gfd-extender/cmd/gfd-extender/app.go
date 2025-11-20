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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/internal/server"
)

const (
	defaultListenAddr       = "0.0.0.0:2376"
	defaultPath             = "/api/v1/detect/gpu"
	defaultShutdownTimeout  = 5 * time.Second
	defaultCollectorTimeout = time.Second
	defaultLogLevel         = "info"
)

type configLoader func(interface{}) error

type detectorFactory func(time.Duration) (closableDetector, error)

type serverFactory func(server.Config, server.Detector, *slog.Logger) serverRunner

type serverRunner interface {
	Run(context.Context) error
}

type closableDetector interface {
	server.Detector
	Close() error
}

type config struct {
	ListenAddr      string        `env:"GFD_EXTENDER_ADDR"`
	Path            string        `env:"GFD_EXTENDER_PATH"`
	Timeout         time.Duration `env:"GFD_EXTENDER_TIMEOUT"`
	ShutdownTimeout time.Duration `env:"GFD_EXTENDER_SHUTDOWN_TIMEOUT"`
	LogLevel        string        `env:"GFD_EXTENDER_LOG_LEVEL" env-default:"info"`
}

func loadConfig(loader configLoader) (config, error) {
	cfg := config{
		ListenAddr:      defaultListenAddr,
		Path:            defaultPath,
		Timeout:         defaultCollectorTimeout,
		ShutdownTimeout: defaultShutdownTimeout,
		LogLevel:        defaultLogLevel,
	}
	if loader == nil {
		loader = func(interface{}) error { return nil }
	}
	if err := loader(&cfg); err != nil {
		return config{}, fmt.Errorf("read environment: %w", err)
	}
	if cfg.ListenAddr == "" {
		return config{}, errors.New("listen address must be set")
	}
	if cfg.Path == "" {
		return config{}, errors.New("HTTP path must be set")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultCollectorTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	return cfg, nil
}

func run(ctx context.Context, log *slog.Logger, cfg config, detectorFn detectorFactory, serverFn serverFactory) error {
	detector, err := detectorFn(cfg.Timeout)
	if err != nil {
		return fmt.Errorf("init detector: %w", err)
	}
	defer func() {
		if err := detector.Close(); err != nil {
			log.Warn("shutdown NVML", slog.String("error", err.Error()))
		}
	}()

	srv := serverFn(server.Config{
		ListenAddr:      cfg.ListenAddr,
		Path:            cfg.Path,
		ShutdownTimeout: cfg.ShutdownTimeout,
	}, detector, log)

	if srv == nil {
		return errors.New("server factory returned nil")
	}

	if err := srv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func newLogger(level string) (*slog.Logger, error) {
	lvl, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})), nil
}

func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level: %s", level)
	}
}
