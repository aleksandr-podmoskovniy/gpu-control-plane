//go:build linux && cgo

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

package detect

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	sysfsPCIDevicesPath = "/sys/bus/pci/devices"
)

var (
	pciDevicesRoot = sysfsPCIDevicesPath
	readSysfsFile  = os.ReadFile
)

func initNVML() error {
	if msg := describeNVMLPresence(); msg != "" {
		fmt.Println(msg)
	}
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return fmt.Errorf("initialize NVML: ret=%d", ret)
	}
	return nil
}

func shutdownNVML() error {
	if ret := nvml.Shutdown(); ret != nvml.SUCCESS {
		return fmt.Errorf("shutdown NVML: ret=%d", ret)
	}
	return nil
}

func describeNVMLPresence() string {
	paths := nvmlSearchPaths()
	found := make([]string, 0, len(paths))
	for _, p := range paths {
		for _, name := range []string{"libnvidia-ml.so.1", "libnvidia-ml.so"} {
			if _, err := os.Stat(filepath.Join(p, name)); err == nil {
				found = append(found, filepath.Join(p, name))
			}
		}
	}
	if len(found) == 0 {
		return fmt.Sprintf("nvml lib not found; searched=%v", paths)
	}
	return fmt.Sprintf("nvml lib candidates=%v", found)
}

func nvmlSearchPaths() []string {
	candidates := []string{
		"/usr/local/nvidia/lib64",
		"/usr/local/nvidia/lib",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib64",
		"/driver-root/usr/lib",
		"/driver-root/usr/lib64",
		"/driver-root/usr/lib/x86_64-linux-gnu",
		"/driver-root/lib",
		"/driver-root/lib64",
		"/lib64",
		"/lib",
	}
	for _, p := range strings.Split(os.Getenv("LD_LIBRARY_PATH"), ":") {
		if p != "" {
			candidates = append([]string{p}, candidates...)
		}
	}
	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		unique = append(unique, p)
	}
	return unique
}

func queryNVML() ([]Info, error) {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get GPU count: %v", nvml.ErrorString(ret))
	}
	infos := make([]Info, 0, count)

	for i := 0; i < count; i++ {
		warnings := warningCollector{}

		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get handle: %v", i, nvml.ErrorString(ret))
			continue
		}

		info := Info{Index: i}
		if info.UUID, ret = device.GetUUID(); ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get uuid: %v", i, nvml.ErrorString(ret))
			continue
		}
		if info.Name, ret = device.GetName(); ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get name: %v", i, nvml.ErrorString(ret))
			continue
		}
		info.Product = info.Name
		mem, ret := device.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get memory info: %v", i, nvml.ErrorString(ret))
		} else {
			info.MemoryInfo = MemoryInfo{
				Total: mem.Total,
				Free:  mem.Free,
				Used:  mem.Used,
			}
		}
		memV2, ret := device.GetMemoryInfo_v2()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get memory v2 info: %v", i, nvml.ErrorString(ret))
		} else {
			info.MemoryInfoV2 = MemoryInfoV2{
				Version:  memV2.Version,
				Total:    memV2.Total,
				Reserved: memV2.Reserved,
				Free:     memV2.Free,
				Used:     memV2.Used,
			}
		}
		if info.PowerUsage, ret = device.GetPowerUsage(); ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get power usage: %v", i, nvml.ErrorString(ret))
		}
		pstate, ret := device.GetPowerState()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get power state: %v", i, nvml.ErrorString(ret))
		}
		info.PowerState = PState(pstate)
		if info.PowerManagementDefaultLimit, ret = device.GetPowerManagementDefaultLimit(); ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get power limit: %v", i, nvml.ErrorString(ret))
		}
		if info.InformImageVersion, ret = device.GetInforomImageVersion(); ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get inforom version: %v", i, nvml.ErrorString(ret))
		}
		processes, ret := device.GetGraphicsRunningProcesses()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get running processes: %v", i, nvml.ErrorString(ret))
		} else {
			info.GraphicsRunningProcesses = make([]ProcessInfo, len(processes))
			for idx, proc := range processes {
				info.GraphicsRunningProcesses[idx] = ProcessInfo{
					Pid:               proc.Pid,
					UsedGpuMemory:     proc.UsedGpuMemory,
					GpuInstanceId:     proc.GpuInstanceId,
					ComputeInstanceId: proc.ComputeInstanceId,
				}
			}
		}

		util, ret := device.GetUtilizationRates()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get utilization: %v", i, nvml.ErrorString(ret))
		} else {
			info.Utilization = Utilization{
				GPU:    util.Gpu,
				Memory: util.Memory,
			}
		}
		info.MemoryMiB = int32(info.MemoryInfo.Total / (1024 * 1024))

		if major, minor, capErr := device.GetCudaComputeCapability(); capErr == nvml.SUCCESS {
			info.ComputeMajor = int32(major)
			info.ComputeMinor = int32(minor)
		} else {
			warnings.addf("gpu %d: get compute capability: %v", i, nvml.ErrorString(capErr))
		}

		if attrs, attrErr := device.GetAttributes(); attrErr == nvml.SUCCESS {
			if attrs.MultiprocessorCount > 0 {
				info.SMCount = ptrInt32(int32(attrs.MultiprocessorCount))
			}
		} else {
			warnings.addf("gpu %d: get attributes: %v", i, nvml.ErrorString(attrErr))
		}

		if memClock, clockErr := device.GetMaxClockInfo(nvml.CLOCK_MEM); clockErr == nvml.SUCCESS {
			if busWidth, bwErr := device.GetMemoryBusWidth(); bwErr == nvml.SUCCESS {
				if bandwidth := calculateMemoryBandwidth(memClock, busWidth); bandwidth != nil {
					info.MemoryBandwidthMiB = bandwidth
				}
			} else {
				warnings.addf("gpu %d: get memory bus width: %v", i, nvml.ErrorString(bwErr))
			}
		} else {
			warnings.addf("gpu %d: get memory clock: %v", i, nvml.ErrorString(clockErr))
		}

		driverVersion, ret := nvml.SystemGetDriverVersion()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get driver version: %v", i, nvml.ErrorString(ret))
		}
		info.DriverVersion = driverVersion

		cudaVersion, ret := nvml.SystemGetCudaDriverVersion()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get cuda version: %v", i, nvml.ErrorString(ret))
		}
		info.CUDADriverVersion = cudaVersion

		pciInfo, ret := device.GetPciInfo()
		if ret != nvml.SUCCESS {
			warnings.addf("gpu %d: get pci info: %v", i, nvml.ErrorString(ret))
		} else {
			info.PCI.Address = formatPCIAddress(pciInfo)
			pciDetails := describePCI(info.PCI.Address, pciInfo)
			warnings.extend(pciDetails.warnings)
			info.PCI.Vendor = pciDetails.vendor
			info.PCI.Device = pciDetails.device
			info.PCI.Class = pciDetails.class
			info.PCI.Subsystem = pciDetails.subsystem
			info.NUMANode = pciDetails.numa
		}

		if gen, genErr := device.GetMaxPcieLinkGeneration(); genErr == nvml.SUCCESS && gen > 0 {
			info.PCIE.Generation = ptrInt32(int32(gen))
		} else if genErr != nvml.SUCCESS {
			warnings.addf("gpu %d: get max pcie link generation: %v", i, nvml.ErrorString(genErr))
		}
		if width, widthErr := device.GetMaxPcieLinkWidth(); widthErr == nvml.SUCCESS && width > 0 {
			info.PCIE.Width = ptrInt32(int32(width))
		} else if widthErr != nvml.SUCCESS {
			warnings.addf("gpu %d: get max pcie link width: %v", i, nvml.ErrorString(widthErr))
		}

		if display, displayErr := device.GetDisplayMode(); displayErr == nvml.SUCCESS {
			info.DisplayMode = displayModeString(display)
		} else {
			warnings.addf("gpu %d: get display mode: %v", i, nvml.ErrorString(displayErr))
		}
		if serial, serialErr := device.GetSerial(); serialErr == nvml.SUCCESS {
			info.Serial = strings.TrimSpace(serial)
		} else {
			warnings.addf("gpu %d: get serial: %v", i, nvml.ErrorString(serialErr))
		}
		if board, boardErr := device.GetBoardPartNumber(); boardErr == nvml.SUCCESS {
			info.Board = strings.TrimSpace(board)
		} else {
			warnings.addf("gpu %d: get board part number: %v", i, nvml.ErrorString(boardErr))
		}
		if arch, archErr := device.GetArchitecture(); archErr == nvml.SUCCESS {
			info.Family = architectureString(arch)
		} else {
			warnings.addf("gpu %d: get architecture: %v", i, nvml.ErrorString(archErr))
		}

		if migMode, _, migErr := device.GetMigMode(); migErr == nvml.SUCCESS {
			info.MIG.Mode = migModeString(migMode)
			if migMode == nvml.DEVICE_MIG_ENABLE {
				info.MIG.Capable = true
			}
		} else {
			warnings.addf("gpu %d: get mig mode: %v", i, nvml.ErrorString(migErr))
		}
		if maxMig, migErr := device.GetMaxMigDeviceCount(); migErr == nvml.SUCCESS && maxMig > 0 {
			info.MIG.Capable = true
		} else if migErr != nvml.SUCCESS {
			warnings.addf("gpu %d: get max mig device count: %v", i, nvml.ErrorString(migErr))
		}
		profiles, migWarnings := collectMigProfiles(device)
		warnings.extend(migWarnings)
		info.MIG.ProfilesSupported = profiles
		if len(info.MIG.ProfilesSupported) > 0 {
			info.MIG.Capable = true
		}

		if len(warnings) > 0 {
			info.Warnings = warnings
			info.Partial = true
		}
		infos = append(infos, info)
	}

	return infos, nil
}

func ptrInt32(v int32) *int32 {
	val := v
	return &val
}

func calculateMemoryBandwidth(memClockMHz, busWidthBits uint32) *int32 {
	if memClockMHz == 0 || busWidthBits == 0 {
		return nil
	}
	clockHz := uint64(memClockMHz) * 1_000_000
	bytesPerSec := clockHz * uint64(busWidthBits) / 8 * 2
	miB := bytesPerSec / (1024 * 1024)
	if miB == 0 {
		return nil
	}
	if miB > math.MaxInt32 {
		miB = math.MaxInt32
	}
	val := int32(miB)
	return &val
}

type pciDetails struct {
	vendor    string
	device    string
	class     string
	subsystem string
	numa      *int32
	warnings  []string
}

func describePCI(address string, info nvml.PciInfo) pciDetails {
	details := pciDetails{}
	if vendor, err := readPCIHex(address, "vendor"); err == nil {
		details.vendor = vendor
	} else {
		details.warnings = append(details.warnings, fmt.Sprintf("pci %s: vendor: %v", address, err))
	}
	if device, err := readPCIHex(address, "device"); err == nil {
		details.device = device
	} else {
		details.warnings = append(details.warnings, fmt.Sprintf("pci %s: device: %v", address, err))
	}
	if classValue, err := readPCIClass(address); err == nil {
		details.class = classValue
	} else {
		details.warnings = append(details.warnings, fmt.Sprintf("pci %s: class: %v", address, err))
	}
	if subsystem, err := readPCISubsystem(address); err == nil {
		details.subsystem = subsystem
	} else {
		details.warnings = append(details.warnings, fmt.Sprintf("pci %s: subsystem: %v", address, err))
	}
	if numa, err := readPCINUMANode(address); err == nil {
		details.numa = numa
	} else {
		details.warnings = append(details.warnings, fmt.Sprintf("pci %s: numa: %v", address, err))
	}

	if details.vendor == "" || details.device == "" {
		vendor, device := splitPCIID(info.PciDeviceId)
		if details.vendor == "" {
			details.vendor = vendor
		}
		if details.device == "" {
			details.device = device
		}
	}
	return details
}

func readPCIHex(address, field string) (string, error) {
	data, err := readPCIFile(address, field)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(strings.ToLower(string(data)))
	value = strings.TrimPrefix(value, "0x")
	return value, nil
}

func readPCIClass(address string) (string, error) {
	value, err := readPCIHex(address, "class")
	if err != nil {
		return "", err
	}
	classVal, err := strconv.ParseInt(value, 16, 32)
	if err != nil {
		return "", err
	}
	classCode := int32(classVal >> 8)
	return fmt.Sprintf("%04x", classCode), nil
}

func readPCISubsystem(address string) (string, error) {
	vendor, err := readPCIHex(address, "subsystem_vendor")
	if err != nil {
		return "", err
	}
	device, err := readPCIHex(address, "subsystem_device")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", vendor, device), nil
}

func readPCINUMANode(address string) (*int32, error) {
	data, err := readPCIFile(address, "numa_node")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	val, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, err
	}
	if val < 0 {
		return nil, nil
	}
	result := int32(val)
	return &result, nil
}

func readPCIFile(address, field string) ([]byte, error) {
	path := filepath.Join(pciDevicesRoot, address, field)
	return readSysfsFile(path)
}

func splitPCIID(id uint32) (vendor, device string) {
	vendor = fmt.Sprintf("%04x", id&0xffff)
	device = fmt.Sprintf("%04x", id>>16)
	return strings.ToLower(vendor), strings.ToLower(device)
}

func formatPCIAddress(info nvml.PciInfo) string {
	function := extractPCIFunction(info.BusId[:])
	address := fmt.Sprintf("%04x:%02x:%02x.%s", info.Domain&0xffff, info.Bus&0xff, info.Device&0xff, function)
	return strings.ToLower(address)
}

func extractPCIFunction(buf []int8) string {
	raw := strings.TrimSpace(cString(buf))
	if idx := strings.LastIndex(raw, "."); idx >= 0 && idx+1 < len(raw) {
		return raw[idx+1:]
	}
	return "0"
}

func cString(buf []int8) string {
	length := 0
	for ; length < len(buf); length++ {
		if buf[length] == 0 {
			break
		}
	}
	bytes := make([]byte, length)
	for i := 0; i < length; i++ {
		bytes[i] = byte(buf[i])
	}
	return string(bytes)
}

func displayModeString(mode nvml.EnableState) string {
	if mode == nvml.FEATURE_ENABLED {
		return "Enabled"
	}
	if mode == nvml.FEATURE_DISABLED {
		return "Disabled"
	}
	return ""
}

func architectureString(arch nvml.DeviceArchitecture) string {
	switch arch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "kepler"
	case nvml.DEVICE_ARCH_MAXWELL:
		return "maxwell"
	case nvml.DEVICE_ARCH_PASCAL:
		return "pascal"
	case nvml.DEVICE_ARCH_VOLTA:
		return "volta"
	case nvml.DEVICE_ARCH_TURING:
		return "turing"
	case nvml.DEVICE_ARCH_AMPERE:
		return "ampere"
	case nvml.DEVICE_ARCH_ADA:
		return "ada"
	case nvml.DEVICE_ARCH_HOPPER:
		return "hopper"
	default:
		return ""
	}
}

func migModeString(mode int) string {
	switch mode {
	case nvml.DEVICE_MIG_ENABLE:
		return "Enabled"
	case nvml.DEVICE_MIG_DISABLE:
		return "Disabled"
	default:
		return ""
	}
}

func collectMigProfiles(device nvml.Device) ([]string, []string) {
	profiles := map[string]struct{}{}
	warnings := warningCollector{}
	for profile := int(nvml.GPU_INSTANCE_PROFILE_1_SLICE); profile < int(nvml.GPU_INSTANCE_PROFILE_COUNT); profile++ {
		infoV := device.GetGpuInstanceProfileInfoV(profile)
		profileInfo, ret := infoV.V2()
		if ret != nvml.SUCCESS {
			warnings.addf("mig profile %d: %v", profile, nvml.ErrorString(ret))
			continue
		}
		if profileInfo.InstanceCount == 0 || profileInfo.SliceCount == 0 {
			continue
		}
		name := cString(profileInfo.Name[:])
		if strings.TrimSpace(name) == "" {
			memGiB := profileInfo.MemorySizeMB / 1024
			if memGiB == 0 {
				continue
			}
			name = fmt.Sprintf("%dg.%dgb", profileInfo.SliceCount, memGiB)
		}
		normalized := strings.ToLower(name)
		if !strings.HasPrefix(normalized, "mig-") {
			normalized = "mig-" + normalized
		}
		profiles[normalized] = struct{}{}
	}
	if len(profiles) == 0 {
		return nil, warnings
	}
	result := make([]string, 0, len(profiles))
	for name := range profiles {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, warnings
}

type warningCollector []string

func (w *warningCollector) addf(format string, args ...interface{}) {
	*w = append(*w, fmt.Sprintf(format, args...))
}

func (w *warningCollector) extend(items []string) {
	if len(items) == 0 {
		return
	}
	*w = append(*w, items...)
}
