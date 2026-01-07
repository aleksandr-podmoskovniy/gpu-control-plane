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
	if len(spec.BindingConditions) > 0 {
		device.BindingConditions = cloneStrings(spec.BindingConditions)
	}
	if len(spec.BindingFailureConditions) > 0 {
		device.BindingFailureConditions = cloneStrings(spec.BindingFailureConditions)
	}
	return device
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
