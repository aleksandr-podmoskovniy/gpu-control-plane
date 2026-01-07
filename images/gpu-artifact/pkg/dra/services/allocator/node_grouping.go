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

import "sort"

func groupByNode(devices []CandidateDevice) map[string][]CandidateDevice {
	nodes := map[string][]CandidateDevice{}
	for _, dev := range devices {
		if dev.NodeName == "" {
			continue
		}
		nodes[dev.NodeName] = append(nodes[dev.NodeName], dev)
	}

	for nodeName := range nodes {
		sort.Slice(nodes[nodeName], func(i, j int) bool {
			return nodes[nodeName][i].Spec.Name < nodes[nodeName][j].Spec.Name
		})
	}

	return nodes
}

func indexDevices(devices []CandidateDevice) map[DeviceKey]CandidateDevice {
	index := make(map[DeviceKey]CandidateDevice, len(devices))
	for _, device := range devices {
		index[device.Key] = device
	}
	return index
}
