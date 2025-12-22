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
	"sort"
	"strings"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func enrichDevicesFromFeature(devices []deviceSnapshot, feature *nfdv1alpha1.NodeFeature) []deviceSnapshot {
	if feature == nil || feature.Spec.Features.Instances == nil {
		return devices
	}

	instanceSet, ok := feature.Spec.Features.Instances["nvidia.com/gpu"]
	if !ok {
		return devices
	}

	indexMap := make(map[string]int, len(devices))
	for i := range devices {
		indexMap[devices[i].Index] = i
	}

	for _, inst := range instanceSet.Elements {
		if inst.Attributes == nil {
			continue
		}
		index := canonicalIndex(inst.Attributes["index"])
		i, ok := indexMap[index]
		if !ok {
			vendor := strings.ToLower(inst.Attributes["vendor"])
			device := strings.ToLower(inst.Attributes["device"])
			class := strings.ToLower(inst.Attributes["class"])
			if vendor == "" || device == "" || class == "" {
				continue
			}
			devices = append(devices, deviceSnapshot{
				Index:  index,
				Vendor: vendor,
				Device: device,
				Class:  class,
			})
			i = len(devices) - 1
			indexMap[index] = i
		}

		if vendor := strings.ToLower(inst.Attributes["vendor"]); vendor != "" && devices[i].Vendor == "" {
			devices[i].Vendor = vendor
		}
		if device := strings.ToLower(inst.Attributes["device"]); device != "" && devices[i].Device == "" {
			devices[i].Device = device
		}
		if class := strings.ToLower(inst.Attributes["class"]); class != "" && devices[i].Class == "" {
			devices[i].Class = class
		}

		if uuid := strings.TrimSpace(inst.Attributes["uuid"]); uuid != "" {
			devices[i].UUID = uuid
		}
		if addr := strings.TrimSpace(inst.Attributes["pci.address"]); addr != "" {
			devices[i].PCIAddress = addr
		}
		if mem := parseMemoryMiB(inst.Attributes["memory.total"]); mem > 0 {
			devices[i].MemoryMiB = mem
		}
		if major := parseInt32(inst.Attributes["compute.major"]); major != 0 {
			devices[i].ComputeMajor = major
		}
		if minor := parseInt32(inst.Attributes["compute.minor"]); minor != 0 {
			devices[i].ComputeMinor = minor
		}
		if product := strings.TrimSpace(inst.Attributes["product"]); product != "" && devices[i].Product == "" {
			devices[i].Product = product
		}
		if numa := parseOptionalInt32(inst.Attributes["numa.node"]); numa != nil {
			devices[i].NUMANode = numa
		}
		if limit := parseOptionalInt32(inst.Attributes["power.limit"]); limit != nil {
			devices[i].PowerLimitMW = limit
		}
		if sm := parseOptionalInt32(inst.Attributes["sm.count"]); sm != nil {
			devices[i].SMCount = sm
		}
		if bw := parseOptionalInt32(inst.Attributes["memory.bandwidth"]); bw != nil {
			devices[i].MemBandwidth = bw
		}
		if gen := parseOptionalInt32(inst.Attributes["pcie.gen"]); gen != nil {
			devices[i].PCIEGen = gen
		}
		if width := parseOptionalInt32(inst.Attributes["pcie.link.width"]); width != nil {
			devices[i].PCIELinkWid = width
		}
		if board := strings.TrimSpace(inst.Attributes["board"]); board != "" && devices[i].Board == "" {
			devices[i].Board = board
		}
		if family := strings.TrimSpace(inst.Attributes["family"]); family != "" && devices[i].Family == "" {
			devices[i].Family = family
		}
		if serial := strings.TrimSpace(inst.Attributes["serial"]); serial != "" && devices[i].Serial == "" {
			devices[i].Serial = serial
		}
		if pstate := strings.TrimSpace(inst.Attributes["pstate"]); pstate != "" && devices[i].PState == "" {
			devices[i].PState = pstate
		}
		if display := strings.TrimSpace(inst.Attributes["display_mode"]); display != "" && devices[i].DisplayMode == "" {
			devices[i].DisplayMode = display
		}

		precisions := extractPrecision(inst.Attributes)
		if len(precisions) > 0 {
			devices[i].Precision = precisions
		}
	}

	sortDeviceSnapshots(devices)
	return devices
}

func extractPrecision(attrs map[string]string) []string {
	var values []string

	if raw := attrs["precision"]; raw != "" {
		values = append(values, splitAndNormalizeList(raw)...)
	}

	for key, value := range attrs {
		if !strings.HasPrefix(key, "precision.") {
			continue
		}
		if parseBool(value) {
			values = append(values, strings.TrimPrefix(key, "precision."))
		}
	}

	if len(values) == 0 {
		return nil
	}

	values = deduplicateStrings(values)
	sort.Strings(values)
	return values
}

func splitAndNormalizeList(input string) []string {
	var result []string
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	for _, f := range fields {
		if trimmed := strings.TrimSpace(f); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func deduplicateStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var out []string
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
