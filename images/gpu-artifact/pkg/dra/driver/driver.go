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
	"path/filepath"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
)

const defaultDriverName = "gpu.deckhouse.io"

// Config defines settings for the DRA kubelet plugin.
type Config struct {
	NodeName            string
	DriverName          string
	KubeClient          kubernetes.Interface
	RegistrarDir        string
	PluginDataRoot      string
	SerializeGRPCCalls  bool
	EnableDebugResponse bool
}

// Driver implements kubeletplugin.DRAPlugin and publishes ResourceSlices.
type Driver struct {
	helper     *kubeletplugin.Helper
	driverName string
	nodeName   string
}

// Start initializes and starts the kubelet DRA plugin.
func Start(ctx context.Context, cfg Config) (*Driver, error) {
	if cfg.KubeClient == nil {
		return nil, errors.New("kube client is required")
	}
	if cfg.NodeName == "" {
		return nil, errors.New("node name is required")
	}
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

	driver := &Driver{
		driverName: driverName,
		nodeName:   cfg.NodeName,
	}

	helper, err := kubeletplugin.Start(
		ctx,
		driver,
		kubeletplugin.KubeClient(cfg.KubeClient),
		kubeletplugin.NodeName(cfg.NodeName),
		kubeletplugin.DriverName(driverName),
		kubeletplugin.Serialize(cfg.SerializeGRPCCalls),
		kubeletplugin.RegistrarDirectoryPath(registrarDir),
		kubeletplugin.PluginDataDirectoryPath(pluginPath),
	)
	if err != nil {
		return nil, err
	}
	driver.helper = helper
	return driver, nil
}

// PublishResources publishes ResourceSlices for this driver.
func (d *Driver) PublishResources(ctx context.Context, resources resourceslice.DriverResources) error {
	if d.helper == nil {
		return errors.New("DRA plugin is not started")
	}
	return d.helper.PublishResources(ctx, resources)
}

// Shutdown stops the DRA plugin.
func (d *Driver) Shutdown() {
	if d.helper != nil {
		d.helper.Stop()
	}
}

// PrepareResourceClaims is a stub until DRA prepare is implemented.
func (d *Driver) PrepareResourceClaims(_ context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	results := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))
	for _, claim := range claims {
		msg := "prepare not implemented"
		if VFIORequested(claim.Annotations) {
			msg = "prepare (vfio requested) not implemented"
		}
		results[claim.UID] = kubeletplugin.PrepareResult{
			Err: fmt.Errorf("%s: %w", msg, kubeletplugin.ErrRecoverable),
		}
	}
	return results, nil
}

// UnprepareResourceClaims is a stub until DRA unprepare is implemented.
func (d *Driver) UnprepareResourceClaims(_ context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	results := make(map[types.UID]error, len(claims))
	for _, claim := range claims {
		results[claim.UID] = nil
	}
	return results, nil
}

// HandleError forwards background errors to the Kubernetes runtime error handler.
func (d *Driver) HandleError(ctx context.Context, err error, msg string) {
	utilruntime.HandleErrorWithContext(ctx, err, msg)
}
