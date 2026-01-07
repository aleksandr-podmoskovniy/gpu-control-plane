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

package prepare

import resourcev1 "k8s.io/api/resource/v1"

type poolIndex struct {
	generation int64
	devices    map[string]resourcev1.Device
}

func indexDevices(slices []resourcev1.ResourceSlice, driverName, nodeName string) map[string]*poolIndex {
	index := map[string]*poolIndex{}
	for _, slice := range slices {
		if driverName != "" && slice.Spec.Driver != driverName {
			continue
		}
		if slice.Spec.NodeName != nil && nodeName != "" && *slice.Spec.NodeName != nodeName {
			continue
		}
		poolName := slice.Spec.Pool.Name
		if poolName == "" {
			continue
		}
		gen := slice.Spec.Pool.Generation
		entry := index[poolName]
		if entry == nil || gen > entry.generation {
			entry = &poolIndex{generation: gen, devices: map[string]resourcev1.Device{}}
			index[poolName] = entry
		}
		if gen < entry.generation {
			continue
		}
		for _, dev := range slice.Spec.Devices {
			entry.devices[dev.Name] = dev
		}
	}
	return index
}
