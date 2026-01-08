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
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func computeConsumedCapacity(req *CapacityRequirements, capacity map[string]allocatable.CapacityValue) (map[string]resource.Quantity, bool) {
	if len(capacity) == 0 {
		if req == nil || len(req.Requests) == 0 {
			return nil, true
		}
		return nil, false
	}

	if req != nil && len(req.Requests) > 0 {
		for name := range req.Requests {
			if _, ok := capacity[name]; !ok {
				return nil, false
			}
		}
	}

	consumed := make(map[string]resource.Quantity, len(capacity))
	for name, cap := range capacity {
		var requested *resource.Quantity
		if req != nil && req.Requests != nil {
			if value, ok := req.Requests[name]; ok {
				requested = &value
			}
		}
		consumedValue, ok := calculateConsumedValue(requested, cap)
		if !ok || consumedValue > cap.Value {
			return nil, false
		}
		consumed[name] = quantityFromValue(consumedValue, cap.Unit)
	}

	return consumed, true
}

func fitsCapacity(existing map[string]resource.Quantity, add map[string]resource.Quantity, capacity map[string]allocatable.CapacityValue) bool {
	for name, cap := range capacity {
		total := quantityValue(add[name], cap.Unit)
		if current, ok := existing[name]; ok {
			total += quantityValue(current, cap.Unit)
		}
		if total > cap.Value {
			return false
		}
	}
	return true
}

func addConsumed(existing map[string]resource.Quantity, add map[string]resource.Quantity, capacity map[string]allocatable.CapacityValue) map[string]resource.Quantity {
	if existing == nil {
		existing = map[string]resource.Quantity{}
	}
	for name, cap := range capacity {
		total := quantityValue(add[name], cap.Unit)
		if current, ok := existing[name]; ok {
			total += quantityValue(current, cap.Unit)
		}
		existing[name] = quantityFromValue(total, cap.Unit)
	}
	return existing
}
