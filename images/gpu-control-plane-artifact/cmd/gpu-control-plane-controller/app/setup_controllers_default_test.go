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

package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestSetupControllersDefaultBranches(t *testing.T) {
	origModule := setupModuleConfigController
	origInventory := setupInventoryController
	origBootstrap := setupBootstrapController
	origGPUPool := setupGPUPoolController
	origClusterPool := setupClusterGPUPoolController
	origGPUPoolUsage := setupGPUPoolUsageController
	origClusterUsage := setupClusterPoolUsageController

	t.Cleanup(func() {
		setupModuleConfigController = origModule
		setupInventoryController = origInventory
		setupBootstrapController = origBootstrap
		setupGPUPoolController = origGPUPool
		setupClusterGPUPoolController = origClusterPool
		setupGPUPoolUsageController = origGPUPoolUsage
		setupClusterPoolUsageController = origClusterUsage
	})

	sysCfg := config.DefaultSystem()
	store := moduleconfig.NewModuleConfigStore(moduleconfig.DefaultState())
	mgr := newFakeManager()

	type testCase struct {
		name      string
		failAt    string
		wantCalls []string
	}

	cases := []testCase{
		{name: "fails-moduleconfig", failAt: "moduleconfig", wantCalls: []string{"moduleconfig"}},
		{name: "fails-inventory", failAt: "inventory", wantCalls: []string{"moduleconfig", "inventory"}},
		{name: "fails-bootstrap", failAt: "bootstrap", wantCalls: []string{"moduleconfig", "inventory", "bootstrap"}},
		{name: "fails-gpupool", failAt: "gpupool", wantCalls: []string{"moduleconfig", "inventory", "bootstrap", "gpupool"}},
		{name: "fails-clustergpupool", failAt: "clustergpupool", wantCalls: []string{"moduleconfig", "inventory", "bootstrap", "gpupool", "clustergpupool"}},
		{name: "fails-gpupool-usage", failAt: "gpupool-usage", wantCalls: []string{"moduleconfig", "inventory", "bootstrap", "gpupool", "clustergpupool", "gpupool-usage"}},
		{name: "fails-cluster-pool-usage", failAt: "cluster-pool-usage", wantCalls: []string{"moduleconfig", "inventory", "bootstrap", "gpupool", "clustergpupool", "gpupool-usage", "cluster-pool-usage"}},
		{name: "success", wantCalls: []string{"moduleconfig", "inventory", "bootstrap", "gpupool", "clustergpupool", "gpupool-usage", "cluster-pool-usage"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := make([]string, 0, len(tc.wantCalls))
			errSentinel := errors.New("boom")

			setupModuleConfigController = func(context.Context, ctrl.Manager, logr.Logger, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "moduleconfig")
				if tc.failAt == "moduleconfig" {
					return errSentinel
				}
				return nil
			}
			setupInventoryController = func(context.Context, ctrl.Manager, logr.Logger, config.ControllerConfig, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "inventory")
				if tc.failAt == "inventory" {
					return errSentinel
				}
				return nil
			}
			setupBootstrapController = func(context.Context, ctrl.Manager, logr.Logger, config.ControllerConfig, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "bootstrap")
				if tc.failAt == "bootstrap" {
					return errSentinel
				}
				return nil
			}
			setupGPUPoolController = func(context.Context, ctrl.Manager, logr.Logger, config.ControllerConfig, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "gpupool")
				if tc.failAt == "gpupool" {
					return errSentinel
				}
				return nil
			}
			setupClusterGPUPoolController = func(context.Context, ctrl.Manager, logr.Logger, config.ControllerConfig, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "clustergpupool")
				if tc.failAt == "clustergpupool" {
					return errSentinel
				}
				return nil
			}
			setupGPUPoolUsageController = func(context.Context, ctrl.Manager, logr.Logger, config.ControllerConfig, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "gpupool-usage")
				if tc.failAt == "gpupool-usage" {
					return errSentinel
				}
				return nil
			}
			setupClusterPoolUsageController = func(context.Context, ctrl.Manager, logr.Logger, config.ControllerConfig, *moduleconfig.ModuleConfigStore) error {
				calls = append(calls, "cluster-pool-usage")
				if tc.failAt == "cluster-pool-usage" {
					return errSentinel
				}
				return nil
			}

			err := setupControllersDefault(context.Background(), mgr, sysCfg.Controllers, store)
			if tc.failAt == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if !errors.Is(err, errSentinel) {
				t.Fatalf("expected sentinel error, got %v", err)
			}

			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Fatalf("unexpected call order: %v", calls)
			}
		})
	}
}
