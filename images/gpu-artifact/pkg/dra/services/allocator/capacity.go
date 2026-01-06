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

func cloneQuantities(in map[string]resource.Quantity) map[string]resource.Quantity {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]resource.Quantity, len(in))
	for key, val := range in {
		out[key] = val.DeepCopy()
	}
	return out
}

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

func calculateConsumedValue(requested *resource.Quantity, capacity allocatable.CapacityValue) (int64, bool) {
	if requested == nil {
		if capacity.Policy != nil {
			return capacity.Policy.Default, true
		}
		return capacity.Value, true
	}

	requestedValue := quantityValue(*requested, capacity.Unit)
	if capacity.Policy == nil {
		return requestedValue, true
	}

	if capacity.Policy.Max > 0 && requestedValue > capacity.Policy.Max {
		return 0, false
	}

	consumed := requestedValue
	min := capacity.Policy.Min
	if consumed < min {
		consumed = min
	}
	if capacity.Policy.Step > 0 {
		diff := consumed - min
		if diff < 0 {
			diff = 0
		}
		remainder := diff % capacity.Policy.Step
		if remainder != 0 {
			consumed += capacity.Policy.Step - remainder
		}
	}
	if capacity.Policy.Max > 0 && consumed > capacity.Policy.Max {
		return 0, false
	}
	return consumed, true
}

func quantityValue(qty resource.Quantity, unit allocatable.CapacityUnit) int64 {
	switch unit {
	case allocatable.CapacityUnitMiB:
		return qty.Value() / (1024 * 1024)
	default:
		return qty.Value()
	}
}

func quantityFromValue(value int64, unit allocatable.CapacityUnit) resource.Quantity {
	switch unit {
	case allocatable.CapacityUnitMiB:
		return *resource.NewQuantity(value*1024*1024, resource.BinarySI)
	default:
		return *resource.NewQuantity(value, resource.DecimalSI)
	}
}
