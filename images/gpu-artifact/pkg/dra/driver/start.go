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

package driver

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

// Start initializes and starts the kubelet DRA plugin.
func Start(ctx context.Context, cfg Config) (*Driver, error) {
	if cfg.KubeClient == nil {
		return nil, errors.New("kube client is required")
	}
	if cfg.NodeName == "" {
		return nil, errors.New("node name is required")
	}
	paths, err := resolveStartPaths(cfg)
	if err != nil {
		return nil, err
	}

	hookPath, err := ResolveNvidiaCDIHookPath(cfg.NvidiaCDIHookPath, paths.PluginPath)
	if err != nil {
		return nil, fmt.Errorf("resolve nvidia-cdi-hook: %w", err)
	}

	prepareService, err := buildPrepareService(cfg, paths.DriverName, paths.PluginPath, paths.CDIRoot, hookPath)
	if err != nil {
		return nil, err
	}

	driver := &Driver{
		driverName:          paths.DriverName,
		nodeName:            cfg.NodeName,
		kubeClient:          cfg.KubeClient,
		prepareService:      prepareService,
		deviceStatusEnabled: cfg.DeviceStatusEnabled,
		errorHandler:        cfg.ErrorHandler,
	}

	helper, err := kubeletplugin.Start(
		ctx,
		driver,
		kubeletplugin.KubeClient(cfg.KubeClient),
		kubeletplugin.NodeName(cfg.NodeName),
		kubeletplugin.DriverName(paths.DriverName),
		kubeletplugin.Serialize(cfg.SerializeGRPCCalls),
		kubeletplugin.RegistrarDirectoryPath(paths.RegistrarDir),
		kubeletplugin.PluginDataDirectoryPath(paths.PluginPath),
	)
	if err != nil {
		return nil, err
	}
	driver.helper = helper
	return driver, nil
}
