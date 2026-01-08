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

package gpuhandler

import (
	"context"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/cdi/nvcdi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/driver"
	drafeaturegates "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/featuregates"
	draallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/featuregates"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler/publish"
	cdisvc "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/cdi"
	handlerresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/resourceslice"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func (b *bootstrapService) initFeatureGates(kubeClient kubernetes.Interface, builder *handlerresourceslice.Builder, recorder eventrecord.EventRecorderLogger, notify func()) *featuregates.Service {
	featureGates := featuregates.NewService(b.cfg.NodeName, b.store, b.log, builder, recorder, notify)
	featureGates.ConfigurePartitionableDevices(kubeClient)
	featureGates.ConfigureConsumableCapacity(kubeClient, b.cfg.ConsumableCapacityMode)
	featureGates.ConfigureSharedCountersLayout(kubeClient)
	return featureGates
}

func (b *bootstrapService) resolveDeviceStatus(kubeClient kubernetes.Interface) bool {
	deviceStatusEnabled, source, serverVersion, err := drafeaturegates.ResolveDeviceStatus(kubeClient, b.cfg.DeviceStatusMode)
	if err != nil {
		b.log.Warn("failed to resolve DRA device status support", "mode", b.cfg.DeviceStatusMode, "source", source, "apiserverVersion", serverVersion, logger.SlogErr(err))
	}
	b.log.Info("DRA device status support resolved", "mode", b.cfg.DeviceStatusMode, "enabled", deviceStatusEnabled, "source", source, "apiserverVersion", serverVersion)
	return deviceStatusEnabled
}

func (b *bootstrapService) startDriver(ctx context.Context, kubeClient kubernetes.Interface, deviceStatusEnabled bool, errorHandler func(ctx context.Context, err error, msg string), notify func()) (*driver.Driver, error) {
	return driver.Start(ctx, driver.Config{
		NodeName:            b.cfg.NodeName,
		KubeClient:          kubeClient,
		DriverRoot:          b.cfg.DriverRoot,
		HostDriverRoot:      b.cfg.HostDriverRoot,
		CDIRoot:             b.cfg.CDIRoot,
		NvidiaCDIHookPath:   b.cfg.NvidiaCDIHookPath,
		DeviceStatusEnabled: deviceStatusEnabled,
		ResourcesChanged:    notify,
		ErrorHandler:        errorHandler,
	})
}

func (b *bootstrapService) buildCDISyncer() publish.CDIBaseSyncer {
	hookPath := b.cfg.NvidiaCDIHookPath
	if hookPath == "" {
		pluginPath := filepath.Join(kubeletplugin.KubeletPluginsDir, draallocator.DefaultDriverName)
		resolvedHookPath, resolveErr := driver.ResolveNvidiaCDIHookPath("", pluginPath)
		if resolveErr != nil {
			b.log.Warn("failed to resolve nvidia-cdi-hook path for CDI base spec", logger.SlogErr(resolveErr))
		} else {
			hookPath = resolvedHookPath
		}
	}
	cdiWriter, cdiErr := nvcdi.New(nvcdi.Options{
		DriverName:        draallocator.DefaultDriverName,
		DriverRoot:        b.cfg.DriverRoot,
		HostDriverRoot:    b.cfg.HostDriverRoot,
		CDIRoot:           b.cfg.CDIRoot,
		NvidiaCDIHookPath: hookPath,
	})
	if cdiErr != nil {
		b.log.Warn("failed to init CDI base writer", logger.SlogErr(cdiErr))
		return nil
	}
	return cdisvc.NewBaseSpecSyncer(cdiWriter)
}
