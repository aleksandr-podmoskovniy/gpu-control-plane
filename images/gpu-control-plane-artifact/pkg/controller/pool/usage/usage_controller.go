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

package usage

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/usage/clustergpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/usage/gpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
)

const (
	gpupoolControllerName        = "gpu-pool-usage-controller"
	clusterGPUPoolControllerName = "cluster-gpu-pool-usage-controller"
)

func SetupController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	if err := setupGPUPoolUsageController(ctx, mgr, log, cfg, store); err != nil {
		return err
	}
	if err := setupClusterGPUPoolUsageController(ctx, mgr, log, cfg, store); err != nil {
		return err
	}
	return nil
}

func setupGPUPoolUsageController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("gpupool.usage")
	r := gpupool.NewReconciler(baseLog, cfg, store)

	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}

	c, err := controller.New(gpupoolControllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(baseLog),
		CacheSyncTimeout:        10 * time.Minute,
		NewQueue:                reconciler.NewNamedQueue(reconciler.UsePriorityQueue()),
	})
	if err != nil {
		return err
	}

	if err := r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	baseLog.Info("Initialized GPUPool usage controller")
	return nil
}

func setupClusterGPUPoolUsageController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	cfg config.ControllerConfig,
	store *moduleconfig.ModuleConfigStore,
) error {
	baseLog := log.WithName("cluster-gpupool.usage")
	r := clustergpupool.NewReconciler(baseLog, cfg, store)

	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}

	c, err := controller.New(clusterGPUPoolControllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(baseLog),
		CacheSyncTimeout:        10 * time.Minute,
		NewQueue:                reconciler.NewNamedQueue(reconciler.UsePriorityQueue()),
	})
	if err != nil {
		return err
	}

	if err := r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	baseLog.Info("Initialized ClusterGPUPool usage controller")
	return nil
}
