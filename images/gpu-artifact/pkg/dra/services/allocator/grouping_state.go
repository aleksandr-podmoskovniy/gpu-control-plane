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
