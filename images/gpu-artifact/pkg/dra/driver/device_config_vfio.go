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
	resourceapi "k8s.io/api/resource/v1"

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

	configs, err := decodeDeviceConfigs(claim.Status.Allocation.Devices.Config, driverName)
	if err != nil {
		return false, err
	}

	for _, cfg := range configs {
		if cfg == nil || cfg.Config == nil {
			continue
		}
		if !configApplies(cfg.Requests, requests) {
			continue
		}
		if _, ok := cfg.Config.(*configapi.VfioDeviceConfig); ok {
			return true, nil
		}
	}

	return false, nil
}
