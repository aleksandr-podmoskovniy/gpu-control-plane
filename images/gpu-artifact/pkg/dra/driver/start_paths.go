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
	"fmt"
	"path/filepath"

	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

type startPaths struct {
	DriverName   string
	RegistrarDir string
	PluginRoot   string
	PluginPath   string
	CDIRoot      string
}

func resolveStartPaths(cfg Config) (startPaths, error) {
	driverName := cfg.DriverName
	if driverName == "" {
		driverName = defaultDriverName
	}
	registrarDir := cfg.RegistrarDir
	if registrarDir == "" {
		registrarDir = kubeletplugin.KubeletRegistryDir
	}
	pluginRoot := cfg.PluginDataRoot
	if pluginRoot == "" {
		pluginRoot = kubeletplugin.KubeletPluginsDir
	}
	pluginPath := filepath.Join(pluginRoot, driverName)
	if err := ensureDir(pluginPath); err != nil {
		return startPaths{}, fmt.Errorf("create plugin directory %q: %w", pluginPath, err)
	}
	cdiRoot := cfg.CDIRoot
	if cdiRoot == "" {
		cdiRoot = defaultCDIRoot
	}
	if err := ensureDir(cdiRoot); err != nil {
		return startPaths{}, fmt.Errorf("ensure CDI root %q: %w", cdiRoot, err)
	}
	return startPaths{
		DriverName:   driverName,
		RegistrarDir: registrarDir,
		PluginRoot:   pluginRoot,
		PluginPath:   pluginPath,
		CDIRoot:      cdiRoot,
	}, nil
}
