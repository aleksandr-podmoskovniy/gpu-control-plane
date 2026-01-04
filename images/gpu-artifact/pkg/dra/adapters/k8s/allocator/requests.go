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

	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// BuildRequests compiles claim requests into domain requests.
func BuildRequests(claim *resourcev1.ResourceClaim, classes map[string]*resourcev1.DeviceClass) ([]domainallocator.Request, error) {
	if claim == nil {
		return nil, nil
	}
	if len(claim.Spec.Devices.Constraints) > 0 {
		return nil, fmt.Errorf("device constraints are not supported yet")
	}

	requests := claim.Spec.Devices.Requests
	out := make([]domainallocator.Request, 0, len(requests))

	for _, req := range requests {
		if req.Exactly == nil {
			return nil, fmt.Errorf("firstAvailable requests are not supported yet")
		}
		if req.Exactly.DeviceClassName == "" {
			return nil, fmt.Errorf("request %q has empty deviceClassName", req.Name)
		}
		if req.Exactly.AdminAccess != nil && *req.Exactly.AdminAccess {
			return nil, fmt.Errorf("request %q admin access is not supported yet", req.Name)
		}
		if req.Exactly.Capacity != nil {
			return nil, fmt.Errorf("request %q capacity requirements are not supported yet", req.Name)
		}

		class, ok := classes[req.Exactly.DeviceClassName]
		if !ok {
			return nil, fmt.Errorf("deviceclass %q not found", req.Exactly.DeviceClassName)
		}

		mode := req.Exactly.AllocationMode
		if mode == "" {
			mode = resourcev1.DeviceAllocationModeExactCount
		}
		if mode != resourcev1.DeviceAllocationModeExactCount {
			return nil, fmt.Errorf("request %q allocation mode %q is not supported yet", req.Name, mode)
		}

		count := req.Exactly.Count
		if count == 0 {
			count = 1
		}
		if count < 1 {
			return nil, fmt.Errorf("request %q count must be positive", req.Name)
		}

		selectors, err := compileSelectors(append(class.Spec.Selectors, req.Exactly.Selectors...))
		if err != nil {
			return nil, fmt.Errorf("request %q: %w", req.Name, err)
		}

		out = append(out, domainallocator.Request{
			Name:      req.Name,
			Count:     count,
			Selectors: selectors,
		})
	}

	return out, nil
}
