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

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	domainalloc "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// BuildAllocationResult converts domain allocation into API allocation.
func BuildAllocationResult(claim *resourcev1.ResourceClaim, alloc *domain.AllocationResult, classes map[string]*resourcev1.DeviceClass) (*resourcev1.AllocationResult, error) {
	if claim == nil || alloc == nil {
		return nil, nil
	}

	results := make([]resourcev1.DeviceRequestAllocationResult, 0, len(alloc.Devices))
	for _, dev := range alloc.Devices {
		result := resourcev1.DeviceRequestAllocationResult{
			Request: dev.Request,
			Driver:  dev.Driver,
			Pool:    dev.Pool,
			Device:  dev.Device,
		}
		if len(dev.ConsumedCapacity) > 0 {
			result.ConsumedCapacity = consumedCapacity(dev.ConsumedCapacity)
			if dev.ShareID != "" {
				shareID := types.UID(dev.ShareID)
				result.ShareID = &shareID
			} else {
				shareID := uuid.NewUUID()
				result.ShareID = &shareID
			}
		}
		results = append(results, result)
	}

	out := &resourcev1.AllocationResult{
		Devices: resourcev1.DeviceAllocationResult{
			Results: results,
		},
		NodeSelector: nodeSelectorForNode(alloc.NodeName),
	}

	config, err := buildAllocationConfig(claim, classes)
	if err != nil {
		return nil, err
	}
	out.Devices.Config = config
	return out, nil
}

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

func consumedCapacity(in map[string]resource.Quantity) map[resourcev1.QualifiedName]resource.Quantity {
	if len(in) == 0 {
		return nil
	}
	out := make(map[resourcev1.QualifiedName]resource.Quantity, len(in))
	for key, val := range in {
		out[resourcev1.QualifiedName(key)] = val.DeepCopy()
	}
	return out
}

func nodeSelectorForNode(nodeName string) *corev1.NodeSelector {
	if nodeName == "" {
		return nil
	}
	return &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchFields: []corev1.NodeSelectorRequirement{{
				Key:      "metadata.name",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{nodeName},
			}},
		}},
	}
}

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
