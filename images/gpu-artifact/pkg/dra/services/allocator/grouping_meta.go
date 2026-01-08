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
	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func deviceMetaFromSpec(spec allocatable.DeviceSpec) deviceMeta {
	return deviceMeta{
		groupKey: deviceGroupKey(spec),
		kind:     deviceKindFromSpec(spec),
	}
}

func deviceGroupKey(spec allocatable.DeviceSpec) string {
	if len(spec.Consumes) > 0 && spec.Consumes[0].CounterSet != "" {
		return spec.Consumes[0].CounterSet
	}
	attr, ok := spec.Attributes[allocatable.AttrPCIAddress]
	if !ok || attr.String == nil {
		return ""
	}
	return allocatable.CounterSetNameForPCI(*attr.String)
}

func deviceKindFromSpec(spec allocatable.DeviceSpec) deviceKind {
	attr, ok := spec.Attributes[allocatable.AttrDeviceType]
	if !ok || attr.String == nil {
		return deviceKindUnknown
	}
	switch *attr.String {
	case string(gpuv1alpha1.DeviceTypeMIG):
		return deviceKindMIG
	case string(gpuv1alpha1.DeviceTypePhysical):
		return deviceKindPhysical
	default:
		return deviceKindUnknown
	}
}
