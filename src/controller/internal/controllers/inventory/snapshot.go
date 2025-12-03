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

package inventory

import (
	"math"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	nvidiacatalog "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/hardware/nvidia"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

const (
	deviceNodeIndexKey  = "status.nodeName"
	deviceLabelPrefix   = "gpu.deckhouse.io/device."
	deviceNodeLabelKey  = "gpu.deckhouse.io/node"
	deviceIndexLabelKey = "gpu.deckhouse.io/device-index"

	gfdProductLabel            = "nvidia.com/gpu.product"
	gfdMemoryLabel             = "nvidia.com/gpu.memory"
	gfdComputeMajorLabel       = "nvidia.com/gpu.compute.major"
	gfdComputeMinorLabel       = "nvidia.com/gpu.compute.minor"
	gfdDriverVersionLabel      = "nvidia.com/gpu.driver"
	gfdCudaRuntimeVersionLabel = "nvidia.com/cuda.runtime.version"
	gfdCudaDriverMajorLabel    = "nvidia.com/cuda.driver.major"
	gfdCudaDriverMinorLabel    = "nvidia.com/cuda.driver.minor"
	gfdMigCapableLabel         = "nvidia.com/mig.capable"
	gfdMigStrategyLabel        = "nvidia.com/mig.strategy"
	gfdMigAltCapableLabel      = "nvidia.com/mig-capable"
	gfdMigAltStrategy          = "nvidia.com/mig-strategy"
	deckhouseToolkitInstalled  = "gpu.deckhouse.io/toolkit.installed"
	deckhouseToolkitReadyLabel = "gpu.deckhouse.io/toolkit.ready"

	migProfileLabelPrefix = "nvidia.com/mig-"
	vendorNvidia          = "10de"
)

type nodeSnapshot struct {
	Managed         bool
	FeatureDetected bool
	Driver          nodeDriverSnapshot
	Devices         []deviceSnapshot
	Labels          map[string]string
}

type nodeDriverSnapshot struct {
	Version          string
	CUDAVersion      string
	ToolkitInstalled bool
	ToolkitReady     bool
}

type deviceSnapshot struct {
	Index        string
	Vendor       string
	Device       string
	Class        string
	PCIAddress   string
	Product      string
	MemoryMiB    int32
	ComputeMajor int32
	ComputeMinor int32
	UUID         string
	Precision    []string
	NUMANode     *int32
	PowerLimitMW *int32
	SMCount      *int32
	MemBandwidth *int32
	PCIEGen      *int32
	PCIELinkWid  *int32
	Board        string
	Family       string
	Serial       string
	PState       string
	DisplayMode  string
	MIG          v1alpha1.GPUMIGConfig
}

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

func extractDeviceSnapshots(labels map[string]string) []deviceSnapshot {
	devices := make(map[string]deviceSnapshot)
	for key, value := range labels {
		if !strings.HasPrefix(key, deviceLabelPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(key, deviceLabelPrefix)
		parts := strings.SplitN(suffix, ".", 2)
		if len(parts) != 2 {
			continue
		}
		index := canonicalIndex(parts[0])
		field := parts[1]

		info := devices[index]
		info.Index = index

		switch field {
		case "vendor":
			info.Vendor = strings.ToLower(value)
		case "device":
			info.Device = strings.ToLower(value)
		case "class":
			info.Class = strings.ToLower(value)
		case "product":
			info.Product = value
		case "memoryMiB":
			info.MemoryMiB = parseMemoryMiB(value)
		}

		devices[index] = info
	}

	// Deduplicate by vendor/device/class only when we have a richer entry (UUID/PCI).
	result := make([]deviceSnapshot, 0, len(devices))
	byUUID := make(map[string]deviceSnapshot)
	byPCI := make(map[string]deviceSnapshot)
	for _, device := range devices {
		if device.Vendor == "" || device.Device == "" || device.Class == "" {
			continue
		}
		if device.Vendor != vendorNvidia {
			continue
		}
		if device.UUID != "" {
			current, ok := byUUID[device.UUID]
			if !ok || scoreDevice(device) > scoreDevice(current) {
				byUUID[device.UUID] = device
			}
			continue
		}
		if device.PCIAddress != "" {
			current, ok := byPCI[device.PCIAddress]
			if !ok || scoreDevice(device) > scoreDevice(current) {
				byPCI[device.PCIAddress] = device
			}
			continue
		}
		result = append(result, device)
	}

	for _, device := range byUUID {
		result = append(result, device)
	}
	for _, device := range byPCI {
		result = append(result, device)
	}

	sortDeviceSnapshots(result)
	return result
}

func scoreDevice(dev deviceSnapshot) int {
	score := 0
	if dev.UUID != "" {
		score += 2
	}
	if dev.PCIAddress != "" {
		score++
	}
	return score
}

func sortDeviceSnapshots(devices []deviceSnapshot) {
	sort.Slice(devices, func(i, j int) bool {
		left := devices[i].Index
		right := devices[j].Index
		if len(left) == len(right) {
			return left < right
		}
		return left < right
	})
}

func parseHardwareDefaults(labels map[string]string) deviceSnapshot {
	snapshot := deviceSnapshot{
		Product:      firstNonEmpty(labels[gfdProductLabel]),
		MemoryMiB:    parseMemoryMiB(labels[gfdMemoryLabel]),
		ComputeMajor: parseInt32(labels[gfdComputeMajorLabel]),
		ComputeMinor: parseInt32(labels[gfdComputeMinorLabel]),
		NUMANode:     parseOptionalInt32(labels["nvidia.com/gpu.numa.node"]),
		PowerLimitMW: parseOptionalInt32(labels["nvidia.com/gpu.power.limit"]),
		SMCount:      parseOptionalInt32(labels["nvidia.com/gpu.sm.count"]),
		MemBandwidth: parseOptionalInt32(labels["nvidia.com/gpu.memory.bandwidth"]),
		PCIEGen:      parseOptionalInt32(labels["nvidia.com/gpu.pcie.gen"]),
		PCIELinkWid:  parseOptionalInt32(labels["nvidia.com/gpu.pcie.link.width"]),
		Board:        strings.TrimSpace(labels["nvidia.com/gpu.board"]),
		Family:       strings.TrimSpace(labels["nvidia.com/gpu.family"]),
		Serial:       strings.TrimSpace(labels["nvidia.com/gpu.serial"]),
		PState:       strings.TrimSpace(labels["nvidia.com/gpu.pstate"]),
		DisplayMode:  strings.TrimSpace(labels["nvidia.com/gpu.display_mode"]),
		MIG:          parseMIGConfig(labels),
	}

	if !snapshot.MIG.Capable && len(snapshot.MIG.Types) > 0 {
		snapshot.MIG.Capable = true
	}

	return snapshot
}

func parseDriverInfo(labels map[string]string) nodeDriverSnapshot {
	driverVersion := strings.TrimSpace(labels[gfdDriverVersionLabel])

	cudaMajor := strings.TrimSpace(labels[gfdCudaDriverMajorLabel])
	cudaMinor := strings.TrimSpace(labels[gfdCudaDriverMinorLabel])
	var cudaVersion string
	switch {
	case cudaMajor != "":
		cudaVersion = cudaMajor
		if cudaMinor != "" {
			cudaVersion += "." + cudaMinor
		}
	case labels[gfdCudaRuntimeVersionLabel] != "":
		cudaVersion = strings.TrimSpace(labels[gfdCudaRuntimeVersionLabel])
	default:
		cudaVersion = ""
	}

	toolkitInstalled := parseBool(labels[deckhouseToolkitInstalled])
	toolkitReady := parseBool(labels[deckhouseToolkitReadyLabel])
	if toolkitReady && !toolkitInstalled {
		toolkitInstalled = true
	}

	return nodeDriverSnapshot{
		Version:          driverVersion,
		CUDAVersion:      cudaVersion,
		ToolkitInstalled: toolkitInstalled,
		ToolkitReady:     toolkitReady,
	}
}

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

func enrichDevicesFromCatalog(devices []deviceSnapshot) {
	for i := range devices {
		if devices[i].Product != "" {
			continue
		}
		if vendor := strings.ToLower(devices[i].Vendor); vendor == "10de" {
			key := vendor + ":" + strings.ToLower(devices[i].Device)
			if name, ok := nvidiacatalog.DeviceNames[key]; ok {
				devices[i].Product = name
			}
		}
	}
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

func canonicalIndex(index string) string {
	index = strings.TrimSpace(index)
	if index == "" {
		return "0"
	}
	if i, err := strconv.Atoi(index); err == nil {
		return strconv.Itoa(i)
	}
	return index
}

func parseMemoryMiB(value string) int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	parts := strings.Fields(value)
	numberPart := parts[0]
	unit := ""
	if len(parts) > 1 {
		unit = strings.ToLower(parts[1])
	}

	floatVal, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		digits := extractLeadingDigits(numberPart)
		if digits == "" {
			return 0
		}
		floatVal, err = strconv.ParseFloat(digits, 64)
		if err != nil {
			return 0
		}
	}

	switch unit {
	case "gib", "gb":
		floatVal *= 1024
	case "tib", "tb":
		floatVal *= 1024 * 1024
	}

	return int32(math.Round(floatVal))
}

func parseInt32(value string) int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		digits := extractLeadingDigits(value)
		if digits == "" {
			return 0
		}
		number, err = strconv.Atoi(digits)
		if err != nil {
			return 0
		}
	}
	return int32(number)
}

func parseOptionalInt32(value string) *int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed := parseInt32(value)
	return &parsed
}

func extractLeadingDigits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		} else {
			break
		}
	}
	return builder.String()
}

func parseMIGConfig(labels map[string]string) v1alpha1.GPUMIGConfig {
	cfg := v1alpha1.GPUMIGConfig{}

	if capableValue, ok := firstExisting(labels, gfdMigCapableLabel, gfdMigAltCapableLabel); ok {
		cfg.Capable = parseBool(capableValue)
	}

	if strategyValue, ok := firstExisting(labels, gfdMigStrategyLabel, gfdMigAltStrategy); ok {
		switch strings.ToLower(strategyValue) {
		case "single":
			cfg.Strategy = v1alpha1.GPUMIGStrategySingle
		case "mixed":
			cfg.Strategy = v1alpha1.GPUMIGStrategyMixed
		default:
			cfg.Strategy = v1alpha1.GPUMIGStrategyNone
		}
	}

	typeAccumulator := map[string]*v1alpha1.GPUMIGTypeCapacity{}
	profiles := map[string]struct{}{}

	for key, value := range labels {
		if !strings.HasPrefix(key, migProfileLabelPrefix) {
			continue
		}

		trimmed := strings.TrimPrefix(key, migProfileLabelPrefix)
		firstDot := strings.Index(trimmed, ".")
		if firstDot == -1 {
			continue
		}
		secondDot := strings.Index(trimmed[firstDot+1:], ".")
		if secondDot == -1 {
			continue
		}
		secondDot += firstDot + 1

		profileCore := trimmed[:secondDot]
		metric := trimmed[secondDot+1:]
		if metric == "" {
			continue
		}

		profileName := "mig-" + profileCore

		count := parseInt32(value)
		if count == 0 && value == "" {
			continue
		}

		entry := typeAccumulator[profileName]
		if entry == nil {
			entry = &v1alpha1.GPUMIGTypeCapacity{Name: profileName}
			typeAccumulator[profileName] = entry
		}

		switch metric {
		case "count", "available", "ready":
			entry.Count = count
		case "memory":
			entry.MemoryMiB = count
		case "multiprocessors":
			entry.Multiprocessors = count
		case "engines.copy":
			entry.Engines.Copy = count
		case "engines.encoder":
			entry.Engines.Encoder = count
		case "engines.decoder":
			entry.Engines.Decoder = count
		case "engines.ofa":
			entry.Engines.OFAs = count
		}

		profiles[profileName] = struct{}{}
	}

	if len(profiles) > 0 {
		cfg.ProfilesSupported = make([]string, 0, len(profiles))
		for profile := range profiles {
			cfg.ProfilesSupported = append(cfg.ProfilesSupported, profile)
		}
		sort.Strings(cfg.ProfilesSupported)
	}

	if len(typeAccumulator) > 0 {
		cfg.Types = make([]v1alpha1.GPUMIGTypeCapacity, 0, len(typeAccumulator))
		for _, entry := range typeAccumulator {
			cfg.Types = append(cfg.Types, *entry)
		}
		sort.Slice(cfg.Types, func(i, j int) bool {
			return cfg.Types[i].Name < cfg.Types[j].Name
		})
	}

	return cfg
}

func migConfigEmpty(cfg v1alpha1.GPUMIGConfig) bool {
	return !cfg.Capable && cfg.Strategy == "" && len(cfg.ProfilesSupported) == 0 && len(cfg.Types) == 0
}

func parseBool(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func firstExisting(labels map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := labels[key]; ok {
			return value, true
		}
	}
	return "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nodeManaged(labels map[string]string, policy ManagedNodesPolicy) bool {
	if val, ok := labels[policy.LabelKey]; ok {
		return !strings.EqualFold(val, "false")
	}
	return policy.EnabledByDefault
}

func sanitizeName(input string) string {
	input = strings.ToLower(input)
	var builder strings.Builder

	lastHyphen := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			builder.WriteRune('-')
			lastHyphen = true
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "gpu"
	}
	return result
}

func truncateName(name string) string {
	const maxLen = 63
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen]
}

func buildDeviceName(nodeName string, info deviceSnapshot) string {
	base := sanitizeName(nodeName)
	suffix := sanitizeName(info.Index + "-" + info.Vendor + "-" + info.Device)
	return truncateName(base + "-" + suffix)
}

func buildInventoryID(nodeName string, info deviceSnapshot) string {
	base := sanitizeName(nodeName)
	suffix := sanitizeName(info.Index + "-" + info.Vendor + "-" + info.Device)
	return truncateName(base + "-" + suffix)
}
