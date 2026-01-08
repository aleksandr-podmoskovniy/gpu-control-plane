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

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func applyDeviceConfigs(req *domain.PrepareRequest, configs []*OpaqueDeviceConfig) error {
	if req == nil {
		return nil
	}
	for i := range req.Devices {
		dev := &req.Devices[i]
		deviceType := allocatable.AttrString(dev.Attributes, allocatable.AttrDeviceType)
		selected, err := defaultConfigForDevice(deviceType)
		if err != nil {
			return err
		}
		for _, cfg := range configs {
			if cfg == nil || cfg.Config == nil {
				continue
			}
			if _, ok := cfg.Config.(*configapi.VfioDeviceConfig); ok {
				continue
			}
			if !configAppliesToRequest(cfg.Requests, dev.Request) {
				continue
			}
			if !configMatchesDeviceType(cfg.Config, deviceType) {
				return fmt.Errorf("cannot apply %T to device %q (type %q)", cfg.Config, dev.Device, deviceType)
			}
			selected = cfg.Config
		}
		dev.Config = selected
	}
	return nil
}

func defaultConfigForDevice(deviceType string) (configapi.Interface, error) {
	switch {
	case allocatable.IsPhysicalDevice(deviceType):
		return configapi.DefaultGpuConfig(), nil
	case allocatable.IsMigDevice(deviceType):
		return configapi.DefaultMigDeviceConfig(), nil
	default:
		return nil, nil
	}
}

func configMatchesDeviceType(cfg configapi.Interface, deviceType string) bool {
	switch cfg.(type) {
	case *configapi.GpuConfig:
		return allocatable.IsPhysicalDevice(deviceType)
	case *configapi.MigDeviceConfig:
		return allocatable.IsMigDevice(deviceType)
	default:
		return false
	}
}
