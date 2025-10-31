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
	"errors"
	"flag"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/manager"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	loadConfigFile = config.LoadFile
	runManager     = manager.Run
	getRESTConfig  = ctrl.GetConfigOrDie
	setupSignals   = ctrl.SetupSignalHandler
)

func main() {
	os.Exit(runMain(os.Args[1:], os.Getenv))
}

func runMain(args []string, getenv func(string) string) int {
	flagSet := flag.NewFlagSet("gpu-control-plane", flag.ExitOnError)
	opts := zap.Options{Development: true}
	opts.BindFlags(flagSet)
	if err := flagSet.Parse(args); err != nil {
		manager.Log.Error(err, "failed to parse flags")
		return 1
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	sysCfg := config.DefaultSystem()
	configPath := getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yaml"
	}

	if configPath != "" {
		if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
			loaded, err := loadConfigFile(configPath)
			if err != nil {
				manager.Log.Error(err, "failed to load config", "path", configPath)
				return 1
			}
			sysCfg = loaded
		} else if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				manager.Log.Error(err, "failed to access config", "path", configPath)
				return 1
			}
			manager.Log.Info("config file not found, using defaults", "path", configPath)
		}
	}

	applyLeaderElectionFromEnv(&sysCfg, getenv)

	restCfg := getRESTConfig()
	ctx := setupSignals()

	if err := runManager(ctx, restCfg, sysCfg); err != nil {
		manager.Log.Error(err, "manager exited with error")
		return 1
	}

	return 0
}

func applyLeaderElectionFromEnv(cfg *config.System, getenv func(string) string) {
	rawEnabled := strings.TrimSpace(getenv("LEADER_ELECTION"))
	if rawEnabled != "" {
		if parsed, err := strconv.ParseBool(rawEnabled); err == nil {
			cfg.LeaderElection.Enabled = parsed
		}
	}

	if ns := strings.TrimSpace(getenv("LEADER_ELECTION_NAMESPACE")); ns != "" {
		cfg.LeaderElection.Namespace = ns
	}

	if id := strings.TrimSpace(getenv("LEADER_ELECTION_ID")); id != "" {
		cfg.LeaderElection.ID = id
	}

	if lock := strings.TrimSpace(getenv("LEADER_ELECTION_RESOURCE_LOCK")); lock != "" {
		cfg.LeaderElection.ResourceLock = lock
	}

	// Provide sensible defaults when enabling leader election via environment variables.
	if cfg.LeaderElection.Enabled {
		if cfg.LeaderElection.ID == "" {
			cfg.LeaderElection.ID = config.DefaultLeaderElectionID
		}
		if cfg.LeaderElection.ResourceLock == "" {
			cfg.LeaderElection.ResourceLock = config.DefaultLeaderElectionResourceLock
		}
		if cfg.LeaderElection.Namespace == "" {
			cfg.LeaderElection.Namespace = strings.TrimSpace(getenv("POD_NAMESPACE"))
		}
	}

	if period := strings.TrimSpace(getenv("INVENTORY_RESYNC_PERIOD")); period != "" {
		if d, err := time.ParseDuration(period); err == nil && d > 0 {
			cfg.Controllers.GPUInventory.ResyncPeriod = d
		}
	}
}
