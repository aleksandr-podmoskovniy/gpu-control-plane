// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package state

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func buildNodeSnapshot(node *corev1.Node, feature *nfdv1alpha1.NodeFeature, policy ManagedNodesPolicy) nodeSnapshot {
	labels := map[string]string{}
	for key, value := range node.Labels {
		labels[key] = value
	}
	if feature != nil {
		for key, value := range feature.Spec.Labels {
			if _, ok := labels[key]; !ok {
				labels[key] = value
			}
		}
	}

	devices := extractDeviceSnapshots(labels)
	defaults := parseHardwareDefaults(labels)

	for i := range devices {
		if devices[i].Product == "" {
			devices[i].Product = defaults.Product
		}
		if devices[i].NUMANode == nil && defaults.NUMANode != nil {
			devices[i].NUMANode = defaults.NUMANode
		}
		if devices[i].PowerLimitMW == nil && defaults.PowerLimitMW != nil {
			devices[i].PowerLimitMW = defaults.PowerLimitMW
		}
		if devices[i].SMCount == nil && defaults.SMCount != nil {
			devices[i].SMCount = defaults.SMCount
		}
		if devices[i].MemBandwidth == nil && defaults.MemBandwidth != nil {
			devices[i].MemBandwidth = defaults.MemBandwidth
		}
		if devices[i].PCIEGen == nil && defaults.PCIEGen != nil {
			devices[i].PCIEGen = defaults.PCIEGen
		}
		if devices[i].PCIELinkWid == nil && defaults.PCIELinkWid != nil {
			devices[i].PCIELinkWid = defaults.PCIELinkWid
		}
		if devices[i].Board == "" {
			devices[i].Board = defaults.Board
		}
		if devices[i].Family == "" {
			devices[i].Family = defaults.Family
		}
		if devices[i].Serial == "" {
			devices[i].Serial = defaults.Serial
		}
		if devices[i].PState == "" {
			devices[i].PState = defaults.PState
		}
		if devices[i].DisplayMode == "" {
			devices[i].DisplayMode = defaults.DisplayMode
		}
		if devices[i].MemoryMiB == 0 {
			devices[i].MemoryMiB = defaults.MemoryMiB
		}
		if devices[i].ComputeMajor == 0 {
			devices[i].ComputeMajor = defaults.ComputeMajor
		}
		if devices[i].ComputeMinor == 0 {
			devices[i].ComputeMinor = defaults.ComputeMinor
		}
		if migConfigEmpty(devices[i].MIG) {
			devices[i].MIG = defaults.MIG
		}
	}
	devices = enrichDevicesFromFeature(devices, feature)
	enrichDevicesFromCatalog(devices)

	return nodeSnapshot{
		Managed:         nodeManaged(labels, policy),
		FeatureDetected: feature != nil,
		Driver:          parseDriverInfo(labels),
		Devices:         devices,
		Labels:          labels,
	}
}

func nodeManaged(labels map[string]string, policy ManagedNodesPolicy) bool {
	if val, ok := labels[policy.LabelKey]; ok {
		return !strings.EqualFold(val, "false")
	}
	return policy.EnabledByDefault
}

