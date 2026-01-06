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

type deviceKind int

const (
	deviceKindUnknown deviceKind = iota
	deviceKindPhysical
	deviceKindMIG
)

type deviceMeta struct {
	groupKey string
	kind     deviceKind
}

type groupState struct {
	hasPhysical bool
	hasMIG      bool
}

func buildDeviceMeta(devices []CandidateDevice) map[DeviceKey]deviceMeta {
	meta := make(map[DeviceKey]deviceMeta, len(devices))
	for _, dev := range devices {
		meta[dev.Key] = deviceMetaFromSpec(dev.Spec)
	}
	return meta
}

func buildGroupState(metaByKey map[DeviceKey]deviceMeta, allocated map[DeviceKey]AllocatedDeviceInfo) map[string]groupState {
	state := map[string]groupState{}
	if len(allocated) == 0 {
		return state
	}
	for key := range allocated {
		meta, ok := metaByKey[key]
		if !ok || meta.groupKey == "" || meta.kind == deviceKindUnknown {
			continue
		}
		st := state[meta.groupKey]
		switch meta.kind {
		case deviceKindMIG:
			st.hasMIG = true
		case deviceKindPhysical:
			st.hasPhysical = true
		}
		state[meta.groupKey] = st
	}
	return state
}

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

func conflictsWithGroup(meta deviceMeta, state map[string]groupState) bool {
	if meta.groupKey == "" || meta.kind == deviceKindUnknown {
		return false
	}
	st := state[meta.groupKey]
	switch meta.kind {
	case deviceKindMIG:
		return st.hasPhysical
	case deviceKindPhysical:
		return st.hasMIG
	default:
		return false
	}
}

func markGroupState(meta deviceMeta, state map[string]groupState) {
	if meta.groupKey == "" || meta.kind == deviceKindUnknown {
		return
	}
	st := state[meta.groupKey]
	switch meta.kind {
	case deviceKindMIG:
		st.hasMIG = true
	case deviceKindPhysical:
		st.hasPhysical = true
	}
	state[meta.groupKey] = st
}
