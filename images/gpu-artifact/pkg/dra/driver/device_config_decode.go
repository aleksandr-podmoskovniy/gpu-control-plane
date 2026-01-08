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
	"strings"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
)

// OpaqueDeviceConfig links opaque parameters to device requests.
type OpaqueDeviceConfig struct {
	Requests []string
	Config   configapi.Interface
}

func decodeDeviceConfigs(configs []resourceapi.DeviceAllocationConfiguration, driverName string) ([]*OpaqueDeviceConfig, error) {
	if len(configs) == 0 {
		return nil, nil
	}

	var classConfigs []resourceapi.DeviceAllocationConfiguration
	var claimConfigs []resourceapi.DeviceAllocationConfiguration
	for _, cfg := range configs {
		switch cfg.Source {
		case resourceapi.AllocationConfigSourceClass:
			classConfigs = append(classConfigs, cfg)
		case resourceapi.AllocationConfigSourceClaim:
			claimConfigs = append(claimConfigs, cfg)
		default:
			return nil, fmt.Errorf("invalid config source: %v", cfg.Source)
		}
	}

	candidates := append(classConfigs, claimConfigs...)
	result := make([]*OpaqueDeviceConfig, 0, len(candidates))
	for _, cfg := range candidates {
		if cfg.Opaque == nil {
			return nil, fmt.Errorf("only opaque configs are supported")
		}
		if cfg.Opaque.Driver != driverName {
			continue
		}
		decoded, err := runtime.Decode(configapi.StrictDecoder, cfg.Opaque.Parameters.Raw)
		if err != nil {
			return nil, fmt.Errorf("decode device config: %w", err)
		}
		config, ok := decoded.(configapi.Interface)
		if !ok {
			return nil, fmt.Errorf("unsupported device config type %T", decoded)
		}
		if err := config.Normalize(); err != nil {
			return nil, fmt.Errorf("normalize device config: %w", err)
		}
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("validate device config: %w", err)
		}
		result = append(result, &OpaqueDeviceConfig{
			Requests: cfg.Requests,
			Config:   config,
		})
	}
	return result, nil
}

func configApplies(configRequests []string, allocated map[string]struct{}) bool {
	if len(configRequests) == 0 {
		return true
	}
	for request := range allocated {
		if configAppliesToRequest(configRequests, request) {
			return true
		}
	}
	return false
}

func configAppliesToRequest(configRequests []string, request string) bool {
	request = strings.TrimSpace(request)
	if request == "" {
		return false
	}
	if len(configRequests) == 0 {
		return true
	}
	for _, name := range configRequests {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == request {
			return true
		}
		if strings.HasPrefix(request, name+"/") {
			return true
		}
	}
	return false
}
