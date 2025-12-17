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

package poolusage

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func SetupGPUPoolUsageController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("gpupool.usage")
	r := NewGPUPoolUsage(baseLog, cfg, store)
	if err := r.SetupWithManager(ctx, mgr); err != nil {
		return err
	}
	baseLog.Info("Initialized GPUPool usage controller")
	return nil
}

func SetupClusterGPUPoolUsageController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("cluster-gpupool.usage")
	r := NewClusterGPUPoolUsage(baseLog, cfg, store)
	if err := r.SetupWithManager(ctx, mgr); err != nil {
		return err
	}
	baseLog.Info("Initialized ClusterGPUPool usage controller")
	return nil
}

