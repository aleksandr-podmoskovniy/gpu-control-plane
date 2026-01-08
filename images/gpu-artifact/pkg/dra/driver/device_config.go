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

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
)

func vfioRequestedFromConfig(claim *resourceapi.ResourceClaim, driverName string) (bool, error) {
	if claim == nil || claim.Status.Allocation == nil {
		return false, nil
	}

	requests := map[string]struct{}{}
	for _, result := range claim.Status.Allocation.Devices.Results {
		if result.Driver != driverName {
			continue
		}
		requests[result.Request] = struct{}{}
	}
	if len(requests) == 0 {
		return false, nil
	}

	for _, cfg := range claim.Status.Allocation.Devices.Config {
		opaque := cfg.Opaque
		if opaque == nil {
			continue
		}
		if opaque.Driver != driverName {
			continue
		}
		if !configApplies(cfg.Requests, requests) {
			continue
		}

		decoded, err := runtime.Decode(configapi.StrictDecoder, opaque.Parameters.Raw)
		if err != nil {
			return false, fmt.Errorf("decode device config: %w", err)
		}
		config, ok := decoded.(configapi.Interface)
		if !ok {
			return false, fmt.Errorf("unsupported device config type %T", decoded)
		}
		if err := config.Normalize(); err != nil {
			return false, fmt.Errorf("normalize device config: %w", err)
		}
		if err := config.Validate(); err != nil {
			return false, fmt.Errorf("validate device config: %w", err)
		}
		if _, ok := decoded.(*configapi.VfioDeviceConfig); ok {
			return true, nil
		}
	}

	return false, nil
}

func configApplies(configRequests []string, allocated map[string]struct{}) bool {
	if len(configRequests) == 0 {
		return true
	}
	for _, name := range configRequests {
		if _, ok := allocated[name]; ok {
			return true
		}
	}
	return false
}
