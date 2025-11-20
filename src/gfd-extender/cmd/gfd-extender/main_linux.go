//go:build linux && cgo

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
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ilyakaznacheev/cleanenv"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/internal/server"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/src/gfd-extender/pkg/detect"
)

func main() {
	cfg, err := loadConfig(readEnvConfig)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})).
			Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log, err := newLogger(cfg.LogLevel)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})).
			Error("invalid log level", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := run(ctx, log, cfg, realDetectorFactory, realServerFactory); err != nil {
		log.Error("gfd-extender failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func realDetectorFactory(timeout time.Duration) (closableDetector, error) {
	client := detect.NewClient(detect.WithTimeout(timeout))
	if err := client.Init(); err != nil {
		return nil, err
	}
	return client, nil
}

func realServerFactory(cfg server.Config, det server.Detector, log *slog.Logger) serverRunner {
	return server.New(cfg, det, log)
}

func readEnvConfig(target interface{}) error {
	return cleanenv.ReadEnv(target)
}
