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
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
)

func TestRunMainUsesDefaultsWhenConfigMissing(t *testing.T) {
	origLoad := loadConfigFile
	origRun := runManager
	origGet := getRESTConfig
	origSetup := setupSignals

	t.Cleanup(func() {
		loadConfigFile = origLoad
		runManager = origRun
		getRESTConfig = origGet
		setupSignals = origSetup
	})

	loadConfigFile = func(path string) (config.System, error) {
		t.Fatalf("unexpected config load for path %s", path)
		return config.DefaultSystem(), nil
	}

	runCalled := false
	runManager = func(ctx context.Context, cfg *rest.Config, sysCfg config.System) error {
		runCalled = true
		if cfg == nil {
			t.Fatal("rest config should not be nil")
		}
		if sysCfg.Controllers.GPUInventory.Workers == 0 {
			t.Fatal("defaults must set workers")
		}
		return nil
	}

	getRESTConfig = func() *rest.Config {
		return &rest.Config{}
	}

	setupSignals = func() context.Context {
		return context.Background()
	}

	code := runMain(nil, func(string) string { return "" })
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !runCalled {
		t.Fatal("runManager was not invoked")
	}
}

func TestRunMainFailsWhenConfigLoadErrors(t *testing.T) {
	origLoad := loadConfigFile
	origRun := runManager
	origGet := getRESTConfig
	origSetup := setupSignals

	t.Cleanup(func() {
		loadConfigFile = origLoad
		runManager = origRun
		getRESTConfig = origGet
		setupSignals = origSetup
	})

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("controllers: {}"), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	loadConfigFile = func(path string) (config.System, error) {
		if path != configPath {
			t.Fatalf("unexpected config path: %s", path)
		}
		return config.System{}, errors.New("broken")
	}
	getRESTConfig = func() *rest.Config { return &rest.Config{} }
	runManager = func(context.Context, *rest.Config, config.System) error { return nil }
	setupSignals = func() context.Context { return context.Background() }

	code := runMain(nil, func(string) string { return configPath })
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRunMainSetsLeaderElectionFromEnv(t *testing.T) {
	origLoad := loadConfigFile
	origRun := runManager
	origGet := getRESTConfig
	origSetup := setupSignals

	t.Cleanup(func() {
		loadConfigFile = origLoad
		runManager = origRun
		getRESTConfig = origGet
		setupSignals = origSetup
	})

	envs := map[string]string{
		"LEADER_ELECTION":               "true",
		"LEADER_ELECTION_NAMESPACE":     "custom-ns",
		"LEADER_ELECTION_ID":            "custom-id",
		"LEADER_ELECTION_RESOURCE_LOCK": "endpointsleases",
		"INVENTORY_RESYNC_PERIOD":       "45s",
	}

	loadConfigFile = func(string) (config.System, error) {
		return config.DefaultSystem(), nil
	}

	getRESTConfig = func() *rest.Config { return &rest.Config{} }
	setupSignals = func() context.Context { return context.Background() }

	envFunc := func(key string) string {
		if val, ok := envs[key]; ok {
			return val
		}
		return ""
	}

	var received config.System
	runManager = func(ctx context.Context, cfg *rest.Config, sysCfg config.System) error {
		received = sysCfg
		return nil
	}

	code := runMain(nil, envFunc)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	if !received.LeaderElection.Enabled {
		t.Fatalf("expected leader election enabled")
	}
	if received.LeaderElection.Namespace != "custom-ns" {
		t.Fatalf("unexpected namespace: %s", received.LeaderElection.Namespace)
	}
	if received.LeaderElection.ID != "custom-id" {
		t.Fatalf("unexpected id: %s", received.LeaderElection.ID)
	}
	if received.LeaderElection.ResourceLock != "endpointsleases" {
		t.Fatalf("unexpected resource lock: %s", received.LeaderElection.ResourceLock)
	}
	if received.Controllers.GPUInventory.ResyncPeriod != 45*time.Second {
		t.Fatalf("unexpected resync period: %s", received.Controllers.GPUInventory.ResyncPeriod)
	}
}
