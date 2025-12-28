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

package physicalgpu

import (
	"context"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/handler"
	internalindexer "github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/indexer"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	physicalgpumetrics "github.com/aleksandr-podmoskovniy/gpu/pkg/monitoring/metrics/physicalgpu"
)

const (
	ControllerName = "physicalgpu-controller"
)

func boolPtr(v bool) *bool {
	return &v
}

// SetupController wires the PhysicalGPU controller using the virtualization-style pattern.
func SetupController(ctx context.Context, mgr manager.Manager, log *log.Logger) error {
	validator := service.NewValidator(mgr.GetClient(), namespaceFromEnv())

	internalindexer.Register()
	if err := indexer.IndexALL(ctx, mgr); err != nil {
		return err
	}

	handlers := []Handler{
		handler.NewValidatorHandler(validator),
	}

	r := NewReconciler(mgr.GetClient(), handlers...)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     boolPtr(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: boolPtr(true),
	})
	if err != nil {
		return err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	physicalgpumetrics.SetupCollector(mgr.GetCache(), metrics.Registry, log)
	return nil
}

func namespaceFromEnv() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	return "d8-gpu-control-plane"
}
