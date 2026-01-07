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

package dra

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"k8s.io/client-go/kubernetes"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/service"
	k8sallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/allocator"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/featuregates"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const (
	ControllerName = "gpu-dra-controller"
)

func boolPtr(v bool) *bool {
	return &v
}

// SetupController wires the DRA allocator controller using the virtualization-style pattern.
func SetupController(ctx context.Context, mgr manager.Manager, log *log.Logger, cfg Config) error {
	allocator := service.NewAllocator(mgr.GetClient())

	deviceStatusMode := cfg.DeviceStatusMode
	if deviceStatusMode == "" {
		deviceStatusMode = "auto"
	}

	var kubeClient kubernetes.Interface
	if mgr.GetConfig() != nil {
		kubeClient, _ = kubernetes.NewForConfig(mgr.GetConfig())
	}
	deviceStatusEnabled, source, serverVersion, err := featuregates.ResolveDeviceStatus(kubeClient, deviceStatusMode)
	if err != nil && log != nil {
		log.Warn("failed to resolve DRA device status support", "mode", deviceStatusMode, "source", source, "apiserverVersion", serverVersion, logger.SlogErr(err))
	}
	if log != nil {
		log.Info("DRA device status support resolved", "mode", deviceStatusMode, "enabled", deviceStatusEnabled, "source", source, "apiserverVersion", serverVersion)
	}
	allocator.SetAllocationOptions(k8sallocator.AllocationOptions{
		IncludeBindingConditions:   deviceStatusEnabled,
		IncludeAllocationTimestamp: deviceStatusEnabled,
	})
	classes := service.NewDeviceClassService(mgr.GetClient())
	writer := service.NewAllocationWriter(mgr.GetClient(), ControllerName)
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName).
		WithLogging(log.With(logger.SlogController(ControllerName)))

	handlers := []Handler{
		handler.NewFeatureGateHandler(classes, recorder),
		handler.NewAllocateHandler(allocator, recorder),
		handler.NewPersistHandler(writer, recorder),
	}

	r := NewReconciler(mgr.GetClient(), handlers...)

	ctr, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     boolPtr(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: boolPtr(true),
	})
	if err != nil {
		return err
	}

	return r.SetupController(ctx, mgr, ctr)
}
