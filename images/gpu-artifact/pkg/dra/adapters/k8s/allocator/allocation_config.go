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

package allocator

import (
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
)

func buildAllocationConfig(claim *resourcev1.ResourceClaim, classes map[string]*resourcev1.DeviceClass) ([]resourcev1.DeviceAllocationConfiguration, error) {
	var cfg []resourcev1.DeviceAllocationConfiguration
	if claim == nil {
		return nil, nil
	}

	requests := claim.Spec.Devices.Requests
	if len(requests) == 0 {
		return nil, nil
	}

	for _, claimCfg := range claim.Spec.Devices.Config {
		cfg = append(cfg, resourcev1.DeviceAllocationConfiguration{
			Source:              resourcev1.AllocationConfigSourceClaim,
			Requests:            claimCfg.Requests,
			DeviceConfiguration: claimCfg.DeviceConfiguration,
		})
	}

	for _, req := range requests {
		if req.Exactly == nil {
			continue
		}
		className := req.Exactly.DeviceClassName
		if className == "" {
			continue
		}
		class, ok := classes[className]
		if !ok {
			return nil, fmt.Errorf("deviceclass %q not found", className)
		}
		for _, classCfg := range class.Spec.Config {
			cfg = append(cfg, resourcev1.DeviceAllocationConfiguration{
				Source:              resourcev1.AllocationConfigSourceClass,
				Requests:            []string{req.Name},
				DeviceConfiguration: classCfg.DeviceConfiguration,
			})
		}
	}

	return cfg, nil
}
