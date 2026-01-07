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
	"sort"

	resourcev1 "k8s.io/api/resource/v1"

	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// BuildCandidates converts resource slices into allocator candidates.
func BuildCandidates(driverName string, slices []resourcev1.ResourceSlice) []domainallocator.CandidateDevice {
	valid := filterPoolSlices(driverName, slices)
	out := make([]domainallocator.CandidateDevice, 0)

	for _, slice := range valid {
		for _, device := range slice.Spec.Devices {
			nodeName, ok := deviceNodeName(slice, device)
			if !ok {
				continue
			}
			key := domainallocator.DeviceKey{Driver: slice.Spec.Driver, Pool: slice.Spec.Pool.Name, Device: device.Name}
			out = append(out, domainallocator.CandidateDevice{
				Key:      key,
				Driver:   slice.Spec.Driver,
				Pool:     slice.Spec.Pool.Name,
				NodeName: nodeName,
				Spec:     toDeviceSpec(device),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeName == out[j].NodeName {
			return out[i].Spec.Name < out[j].Spec.Name
		}
		return out[i].NodeName < out[j].NodeName
	})

	return out
}
