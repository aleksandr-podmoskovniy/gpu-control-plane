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

package allocatable

import (
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	domain "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// DeviceRenderOptions controls rendering of optional device fields.
type DeviceRenderOptions struct {
	IncludeCapacity         bool
	IncludeMultiAllocations bool
}

// RenderDevices converts domain devices into API devices.
func RenderDevices(devices []domain.Device) []resourceapi.Device {
	return RenderDevicesWithOptions(devices, DeviceRenderOptions{
		IncludeCapacity:         true,
		IncludeMultiAllocations: true,
	})
}

// RenderDevicesWithOptions converts domain devices into API devices with options.
func RenderDevicesWithOptions(devices []domain.Device, opts DeviceRenderOptions) []resourceapi.Device {
	if len(devices) == 0 {
		return nil
	}
	out := make([]resourceapi.Device, 0, len(devices))
	for _, dev := range devices {
		out = append(out, renderDeviceWithOptions(dev.Spec(), opts))
	}
	return out
}

// RenderCounterSets converts domain counter sets into API counter sets.
func RenderCounterSets(counterSets []domain.CounterSet) []resourceapi.CounterSet {
	if len(counterSets) == 0 {
		return nil
	}
	out := make([]resourceapi.CounterSet, 0, len(counterSets))
	for _, set := range counterSets {
		out = append(out, resourceapi.CounterSet{
			Name:     set.Name,
			Counters: renderCounters(set.Counters),
		})
	}
	return out
}

func renderDeviceWithOptions(spec domain.DeviceSpec, opts DeviceRenderOptions) resourceapi.Device {
	device := resourceapi.Device{
		Name:       spec.Name,
		Attributes: RenderAttributes(spec.Attributes),
	}
	if opts.IncludeCapacity {
		device.Capacity = RenderCapacities(spec.Capacity)
	}
	if len(spec.Consumes) > 0 {
		device.ConsumesCounters = renderConsumes(spec.Consumes)
	}
	if opts.IncludeMultiAllocations && spec.AllowMultipleAllocations {
		device.AllowMultipleAllocations = ptr.To(true)
	}
	return device
}

// RenderAttributes converts domain attributes into API attributes.
func RenderAttributes(attrs map[string]domain.AttributeValue) map[resourceapi.QualifiedName]resourceapi.DeviceAttribute {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[resourceapi.QualifiedName]resourceapi.DeviceAttribute, len(attrs))
	for key, val := range attrs {
		attr := resourceapi.DeviceAttribute{
			StringValue: val.String,
			IntValue:    val.Int,
			BoolValue:   val.Bool,
		}
		out[resourceapi.QualifiedName(key)] = attr
	}
	return out
}

// RenderCapacities converts domain capacities into API capacities.
func RenderCapacities(capacity map[string]domain.CapacityValue) map[resourceapi.QualifiedName]resourceapi.DeviceCapacity {
	if len(capacity) == 0 {
		return nil
	}
	out := make(map[resourceapi.QualifiedName]resourceapi.DeviceCapacity, len(capacity))
	for key, val := range capacity {
		capacityValue := resourceapi.DeviceCapacity{
			Value: quantityFromCapacity(val.Value, val.Unit),
		}
		if val.Policy != nil {
			capacityValue.RequestPolicy = &resourceapi.CapacityRequestPolicy{
				Default: quantityPtrFromCapacity(val.Policy.Default, val.Policy.Unit),
				ValidRange: &resourceapi.CapacityRequestPolicyRange{
					Min:  quantityPtrFromCapacity(val.Policy.Min, val.Policy.Unit),
					Max:  quantityPtrFromCapacity(val.Policy.Max, val.Policy.Unit),
					Step: quantityPtrFromCapacity(val.Policy.Step, val.Policy.Unit),
				},
			}
		}
		out[resourceapi.QualifiedName(key)] = capacityValue
	}
	return out
}

func renderConsumes(consumes []domain.CounterConsumption) []resourceapi.DeviceCounterConsumption {
	out := make([]resourceapi.DeviceCounterConsumption, 0, len(consumes))
	for _, consumption := range consumes {
		out = append(out, resourceapi.DeviceCounterConsumption{
			CounterSet: consumption.CounterSet,
			Counters:   renderCounters(consumption.Counters),
		})
	}
	return out
}

func renderCounters(values map[string]domain.CounterValue) map[string]resourceapi.Counter {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]resourceapi.Counter, len(values))
	for key, val := range values {
		out[key] = resourceapi.Counter{
			Value: quantityFromCounter(val.Value, val.Unit),
		}
	}
	return out
}

func quantityFromCapacity(value int64, unit domain.CapacityUnit) resource.Quantity {
	switch unit {
	case domain.CapacityUnitMiB:
		return *resource.NewQuantity(value*1024*1024, resource.BinarySI)
	case domain.CapacityUnitPercent, domain.CapacityUnitCount:
		return *resource.NewQuantity(value, resource.DecimalSI)
	default:
		return *resource.NewQuantity(value, resource.DecimalSI)
	}
}

func quantityPtrFromCapacity(value int64, unit domain.CapacityUnit) *resource.Quantity {
	qty := quantityFromCapacity(value, unit)
	return &qty
}

func quantityFromCounter(value int64, unit domain.CounterUnit) resource.Quantity {
	switch unit {
	case domain.CounterUnitMiB:
		return *resource.NewQuantity(value*1024*1024, resource.BinarySI)
	case domain.CounterUnitCount:
		return *resource.NewQuantity(value, resource.DecimalSI)
	default:
		return *resource.NewQuantity(value, resource.DecimalSI)
	}
}
