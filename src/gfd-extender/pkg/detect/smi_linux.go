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

//go:build linux

package detect

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	smiTimeout          = 5 * time.Second
	sysfsPCIDevicesPath = "/sys/bus/pci/devices"

	smiIdxUUID = iota
	smiIdxName
	smiIdxMemTotal
	smiIdxMemFree
	smiIdxMemUsed
	smiIdxPState
	smiIdxClockGraphics
	smiIdxClockMem
	smiIdxUtilGPU
	smiIdxUtilMem
	smiIdxPCIBus
	smiIdxPCIEGen
	smiIdxPCIEWidth
	smiIdxSerial
	smiIdxFamily
	smiIdxTempGPU
	smiIdxPowerDraw
	smiIdxDriverVersion

	smiExpectedFields = 18
)

var (
	pciDevicesRoot = sysfsPCIDevicesPath
	readSysfsFile  = os.ReadFile
	execSmi        = runSmi
)

func initNVML() error { return nil }

func shutdownNVML() error { return nil }

func describeNVMLPresence() string {
	return fmt.Sprintf("nvml lib search paths=%v", nvmlSearchPaths())
}

// queryNVML collects GPU info via nvidia-smi (no cgo/NVML bindings to avoid crashes).
func queryNVML() ([]Info, error) {
	rows, err := execSmi(context.Background())
	if err != nil {
		return nil, err
	}

	infos := make([]Info, 0, len(rows))
	for idx, fields := range rows {
		w := warningCollector{}
		info := Info{Index: idx}

		setStr := func(target *string, i int) {
			if i < len(fields) {
				*target = strings.TrimSpace(fields[i])
			} else {
				w.addf("gpu %d: missing field %d", idx, i)
			}
		}

		setStr(&info.UUID, smiIdxUUID)
		setStr(&info.Name, smiIdxName)
		info.Product = info.Name

		parseMem := func(i int) uint64 {
			if i >= len(fields) {
				return 0
			}
			v, _ := strconv.ParseFloat(strings.TrimSpace(fields[i]), 64)
			return uint64(v * 1024 * 1024)
		}
		total := parseMem(smiIdxMemTotal)
		free := parseMem(smiIdxMemFree)
		used := parseMem(smiIdxMemUsed)
		info.MemoryInfo = MemoryInfo{Total: total, Free: free, Used: used}
		info.MemoryInfoV2 = MemoryInfoV2{Total: total, Free: free, Used: used}
		if total > 0 {
			info.MemoryMiB = int32(total / (1024 * 1024))
		}

		if len(fields) > smiIdxPState {
			ps := strings.TrimPrefix(strings.TrimSpace(fields[smiIdxPState]), "P")
			if v, err := strconv.Atoi(ps); err == nil {
				info.PowerState = PState(v)
			}
		}
		if len(fields) > smiIdxUtilGPU {
			if v, err := strconv.Atoi(strings.TrimSpace(fields[smiIdxUtilGPU])); err == nil {
				info.Utilization.GPU = uint32(v)
			}
		}
		if len(fields) > smiIdxUtilMem {
			if v, err := strconv.Atoi(strings.TrimSpace(fields[smiIdxUtilMem])); err == nil {
				info.Utilization.Memory = uint32(v)
			}
		}
		if len(fields) > smiIdxPCIBus {
			info.PCI.Address = strings.TrimSpace(fields[smiIdxPCIBus])
		}
		if len(fields) > smiIdxPCIEGen {
			if v, err := strconv.Atoi(strings.TrimSpace(fields[smiIdxPCIEGen])); err == nil {
				info.PCIE.Generation = ptrInt32(int32(v))
			}
		}
		if len(fields) > smiIdxPCIEWidth {
			if v, err := strconv.Atoi(strings.TrimSpace(fields[smiIdxPCIEWidth])); err == nil {
				info.PCIE.Width = ptrInt32(int32(v))
			}
		}
		if len(fields) > smiIdxSerial {
			info.Serial = strings.TrimSpace(fields[smiIdxSerial])
		}
		if len(fields) > smiIdxFamily {
			info.Family = strings.TrimSpace(fields[smiIdxFamily])
		}
		if len(fields) > smiIdxPowerDraw {
			if v, err := strconv.ParseFloat(strings.TrimSpace(fields[smiIdxPowerDraw]), 64); err == nil {
				info.PowerUsage = uint32(v * 1000) // W -> mW
			}
		}
		if len(fields) > smiIdxDriverVersion {
			info.DriverVersion = strings.TrimSpace(fields[smiIdxDriverVersion])
		}

		if pciInfo, err := collectPCIInfo(info.PCI.Address, idx); err == nil {
			if info.PCI.Address == "" {
				info.PCI.Address = pciInfo.Address
			}
			if info.PCI.Vendor == "" {
				info.PCI.Vendor = pciInfo.Vendor
			}
			if info.PCI.Device == "" {
				info.PCI.Device = pciInfo.Device
			}
			if info.PCI.Class == "" {
				info.PCI.Class = pciInfo.Class
			}
		} else {
			w.addf("gpu %d: pci info: %v", idx, err)
		}

		if len(w) > 0 {
			info.Partial = true
			info.Warnings = w
		}
		infos = append(infos, info)
	}

	return infos, nil
}

func runSmi(ctx context.Context) ([][]string, error) {
	ctx, cancel := context.WithTimeout(ctx, smiTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=uuid,name,memory.total,memory.free,memory.used,pstate,clocks.gr,clocks.mem,utilization.gpu,utilization.memory,pci.bus_id,pci.link.gen.current,pci.link.width.current,serial,family,temperature.gpu,power.draw,driver_version",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi failed: %w", err)
	}
	lines := bytes.Split(bytes.TrimSpace(out), []byte{'\n'})
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		fields := strings.Split(string(line), ",")
		for len(fields) < smiExpectedFields {
			fields = append(fields, "")
		}
		rows = append(rows, fields)
	}
	return rows, nil
}

func collectPCIInfo(busID string, index int) (PCIInfo, error) {
	pciAddr := normalizePCIAddress(busID)
	info := PCIInfo{Address: pciAddr}
	if pciAddr == "" {
		return info, nil
	}
	sysfsPath := filepath.Join(pciDevicesRoot, pciAddr)
	if _, err := os.Stat(sysfsPath); err != nil {
		return info, nil
	}

	if vendor, err := readHexValue(filepath.Join(sysfsPath, "vendor")); err == nil {
		info.Vendor = vendor
	}
	if device, err := readHexValue(filepath.Join(sysfsPath, "device")); err == nil {
		info.Device = device
	}
	if sysfsClass, err := readHexValue(filepath.Join(sysfsPath, "class")); err == nil {
		info.Class = sysfsClass
	}
	return info, nil
}

func normalizePCIAddress(busID string) string {
	parts := strings.Split(busID, ":")
	switch len(parts) {
	case 2:
		return strings.ToLower(busID)
	case 1:
		if strings.Contains(busID, ".") {
			return strings.ToLower("0000:" + busID)
		}
	}
	return ""
}

func readHexValue(path string) (string, error) {
	data, err := readSysfsFile(path)
	if err != nil {
		return "", err
	}
	val := strings.TrimSpace(string(data))
	val = strings.TrimPrefix(val, "0x")
	return fmt.Sprintf("0x%s", val), nil
}

func nvmlSearchPaths() []string {
	candidates := []string{
		"/usr/local/nvidia/lib64",
		"/usr/local/nvidia/lib",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib64",
		"/driver-root/compat/lib",
		"/driver-root/compat/lib64",
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

func ptrInt32(v int32) *int32 {
	return &v
}
