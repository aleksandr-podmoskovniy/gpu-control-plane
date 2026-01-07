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
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	domainalloc "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func capacityValueFromQuantity(qty resource.Quantity, policy *resourcev1.CapacityRequestPolicy) domainalloc.CapacityValue {
	unit := domainalloc.CapacityUnitCount
	value := qty.Value()
	if qty.Format == resource.BinarySI {
		unit = domainalloc.CapacityUnitMiB
		value = qty.Value() / (1024 * 1024)
	}

	out := domainalloc.CapacityValue{
		Value: value,
		Unit:  unit,
	}
	if policy == nil {
		return out
	}

	out.Policy = &domainalloc.CapacityPolicy{
		Default: quantityValue(policy.Default, unit),
		Unit:    unit,
	}
	if policy.ValidRange != nil {
		out.Policy.Min = quantityValue(policy.ValidRange.Min, unit)
		out.Policy.Max = quantityValue(policy.ValidRange.Max, unit)
		out.Policy.Step = quantityValue(policy.ValidRange.Step, unit)
	}
	return out
}

func counterValueFromQuantity(qty resource.Quantity) domainalloc.CounterValue {
	unit := domainalloc.CounterUnitCount
	value := qty.Value()
	if qty.Format == resource.BinarySI {
		unit = domainalloc.CounterUnitMiB
		value = qty.Value() / (1024 * 1024)
	}
	return domainalloc.CounterValue{
		Value: value,
		Unit:  unit,
	}
}

func quantityValue(qty *resource.Quantity, unit domainalloc.CapacityUnit) int64 {
	if qty == nil {
		return 0
	}
	if unit == domainalloc.CapacityUnitMiB {
		return qty.Value() / (1024 * 1024)
	}
	return qty.Value()
}
