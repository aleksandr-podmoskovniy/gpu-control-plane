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
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/internal/server"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/pkg/detect"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(func(any) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != defaultListenAddr || cfg.Path != defaultPath {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
	if cfg.Timeout != defaultCollectorTimeout || cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("unexpected timeouts: %+v", cfg)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatalf("unexpected log level: %s", cfg.LogLevel)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	cfg, err := loadConfig(func(target interface{}) error {
		out := target.(*config)
		out.ListenAddr = "127.0.0.1:4000"
		out.Path = "/custom"
		out.Timeout = 0
		out.ShutdownTimeout = 0
		out.LogLevel = "debug"
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:4000" || cfg.Path != "/custom" {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
	if cfg.Timeout != defaultCollectorTimeout || cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("timeouts not normalized: %+v", cfg)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("log level override not applied: %s", cfg.LogLevel)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	_, err := loadConfig(func(target interface{}) error {
		out := target.(*config)
		out.ListenAddr = ""
		return nil
	})
	if err == nil {
		t.Fatalf("expected error for empty listen addr")
	}

	_, err = loadConfig(func(target interface{}) error {
		out := target.(*config)
		out.Path = ""
		return nil
	})
	if err == nil {
		t.Fatalf("expected error for empty path")
	}

	expected := errors.New("boom")
	_, err = loadConfig(func(any) error { return expected })
	if !errors.Is(err, expected) {
		t.Fatalf("expected loader error, got %v", err)
	}
}

func TestLoadConfigNilLoader(t *testing.T) {
	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != defaultListenAddr || cfg.Path != defaultPath {
		t.Fatalf("expected defaults for nil loader")
	}
}

func TestLoadConfigResetsEmptyLogLevel(t *testing.T) {
	cfg, err := loadConfig(func(target interface{}) error {
		out := target.(*config)
		out.LogLevel = ""
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatalf("expected log level defaulted, got %s", cfg.LogLevel)
	}
}

func TestParseLogLevel(t *testing.T) {
	level, err := parseLogLevel("debug")
	if err != nil || level != slog.LevelDebug {
		t.Fatalf("expected debug level, got %v err=%v", level, err)
	}
	if _, err := parseLogLevel("bad"); err == nil {
		t.Fatalf("expected error for invalid level")
	}
	if level, err := parseLogLevel(""); err != nil || level != slog.LevelInfo {
		t.Fatalf("expected default info for empty level")
	}
	if level, err := parseLogLevel("warn"); err != nil || level != slog.LevelWarn {
		t.Fatalf("expected warn level, got %v err=%v", level, err)
	}
	if level, err := parseLogLevel("error"); err != nil || level != slog.LevelError {
		t.Fatalf("expected error level, got %v err=%v", level, err)
	}
}

func TestNewLogger(t *testing.T) {
	logger, err := newLogger("warn")
	if err != nil || logger == nil {
		t.Fatalf("expected logger, got err=%v", err)
	}
	if _, err := newLogger("invalid"); err == nil {
		t.Fatalf("expected error for bad log level")
	}
}

type fakeDetector struct {
	closeErr  error
	closed    bool
	detectErr error
}

func (f *fakeDetector) DetectGPU(context.Context) ([]detect.Info, error) {
	if f.detectErr != nil {
		return nil, f.detectErr
	}
	return nil, nil
}

func (f *fakeDetector) Close() error {
	f.closed = true
	return f.closeErr
}

type fakeServer struct {
	err error
}

func (f *fakeServer) Run(context.Context) error {
	return f.err
}

func TestRunSuccess(t *testing.T) {
	det := &fakeDetector{}
	srv := &fakeServer{}
	cfg := config{
		ListenAddr:      "127.0.0.1:0",
		Path:            "/detect",
		Timeout:         defaultCollectorTimeout,
		ShutdownTimeout: defaultShutdownTimeout,
	}
	err := run(context.Background(), discardLogger(), cfg,
		func(time.Duration) (closableDetector, error) { return det, nil },
		func(server.Config, server.Detector, *slog.Logger) serverRunner { return srv },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !det.closed {
		t.Fatalf("detector was not closed")
	}
}

func TestRunDetectorError(t *testing.T) {
	expected := errors.New("init failed")
	err := run(context.Background(), discardLogger(), config{}, func(time.Duration) (closableDetector, error) {
		return nil, expected
	}, nil)
	if !errors.Is(err, expected) {
		t.Fatalf("expected detector error, got %v", err)
	}
}

func TestRunServerError(t *testing.T) {
	expected := errors.New("server failed")
	err := run(context.Background(), discardLogger(), config{}, func(time.Duration) (closableDetector, error) {
		return &fakeDetector{}, nil
	}, func(server.Config, server.Detector, *slog.Logger) serverRunner {
		return &fakeServer{err: expected}
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected server error, got %v", err)
	}
}

func TestRunServerCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := run(ctx, discardLogger(), config{}, func(time.Duration) (closableDetector, error) {
		return &fakeDetector{}, nil
	}, func(server.Config, server.Detector, *slog.Logger) serverRunner {
		return &fakeServer{err: context.Canceled}
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunServerFactoryNil(t *testing.T) {
	err := run(context.Background(), discardLogger(), config{}, func(time.Duration) (closableDetector, error) {
		return &fakeDetector{}, nil
	}, func(server.Config, server.Detector, *slog.Logger) serverRunner { return nil })
	if err == nil {
		t.Fatalf("expected error when server factory returns nil")
	}
}

func TestRunCloseError(t *testing.T) {
	det := &fakeDetector{closeErr: errors.New("close")}
	err := run(context.Background(), discardLogger(), config{}, func(time.Duration) (closableDetector, error) {
		return det, nil
	}, func(server.Config, server.Detector, *slog.Logger) serverRunner {
		return &fakeServer{}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !det.closed {
		t.Fatalf("detector was not closed")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}
