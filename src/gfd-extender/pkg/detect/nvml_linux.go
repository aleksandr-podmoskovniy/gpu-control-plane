//go:build linux

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
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Real NVML-backed implementation.

func initNVML() error {
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return fmt.Errorf("initialize NVML: %s", nvml.ErrorString(ret))
	}
	return nil
}

func shutdownNVML() error {
	if ret := nvml.Shutdown(); ret != nvml.SUCCESS {
		return fmt.Errorf("shutdown NVML: %s", nvml.ErrorString(ret))
	}
	return nil
}

func queryNVML() ([]Info, error) {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get device count: %s", nvml.ErrorString(ret))
	}

	infos := make([]Info, 0, count)
	now := time.Now().UTC()

	for i := 0; i < count; i++ {
		dev, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return infos, fmt.Errorf("get handle %d: %s", i, nvml.ErrorString(ret))
		}

		info := Info{Index: i}

		if uuid, ret := dev.GetUUID(); ret == nvml.SUCCESS {
			info.UUID = uuid
		}
		if name, ret := dev.GetName(); ret == nvml.SUCCESS {
			info.Name = name
			info.Product = name
		}

		if mem, ret := dev.GetMemoryInfo(); ret == nvml.SUCCESS {
			info.MemoryInfo = MemoryInfo{Total: mem.Total, Free: mem.Free, Used: mem.Used}
			info.MemoryInfoV2 = MemoryInfoV2{Total: mem.Total, Free: mem.Free, Used: mem.Used}
			if mem.Total > 0 {
				info.MemoryMiB = int32(mem.Total / (1024 * 1024))
			}
		}
		if pwr, ret := dev.GetPowerUsage(); ret == nvml.SUCCESS {
			info.PowerUsage = pwr
		}
		if lim, ret := dev.GetPowerManagementDefaultLimit(); ret == nvml.SUCCESS {
			info.PowerManagementDefaultLimit = lim
		}
		if ps, ret := dev.GetPowerState(); ret == nvml.SUCCESS {
			info.PowerState = PState(ps)
		}
		if util, ret := dev.GetUtilizationRates(); ret == nvml.SUCCESS {
			info.Utilization = Utilization{GPU: util.Gpu, Memory: util.Memory}
		}
		if ccMajor, ccMinor, ret := dev.GetCudaComputeCapability(); ret == nvml.SUCCESS {
			info.ComputeMajor = int32(ccMajor)
			info.ComputeMinor = int32(ccMinor)
		}
		// SMCount not available in this go-nvml version; leave nil.
		// NUMA node is not available in this NVML binding; keep nil.

		if pci, ret := dev.GetPciInfo(); ret == nvml.SUCCESS {
			busID := strings.TrimRight(string(pci.BusId[:]), "\x00")
			info.PCI = PCIInfo{
				Address: strings.ToLower(busID),
				Vendor:  fmt.Sprintf("%04x", pci.PciDeviceId>>16),
				Device:  fmt.Sprintf("%04x", pci.PciDeviceId&0xffff),
				Class:   "",
			}
		}
		if gen, ret := dev.GetCurrPcieLinkGeneration(); ret == nvml.SUCCESS {
			val := int32(gen)
			info.PCIE.Generation = &val
		}
		if width, ret := dev.GetCurrPcieLinkWidth(); ret == nvml.SUCCESS {
			val := int32(width)
			info.PCIE.Width = &val
		}

		if migMode, _, ret := dev.GetMigMode(); ret == nvml.SUCCESS {
			if migMode != 0 {
				info.MIG.Capable = true
				info.MIG.Mode = fmt.Sprintf("%d", migMode)
			}
		}

		info.Precision = derivePrecisions(info.ComputeMajor, info.ComputeMinor)

		info.Warnings = append(info.Warnings, fmt.Sprintf("updated=%s", now.Format(time.RFC3339)))
		infos = append(infos, info)
	}

	return infos, nil
}

// estimateMemoryBandwidth returns an approximate bandwidth in MiB/s based on memory clock and bus width.
// Not critical; best-effort only.
func estimateMemoryBandwidth(dev nvml.Device) (uint64, error) {
	// Not supported in current go-nvml version; return 0 to skip.
	return 0, fmt.Errorf("memory bandwidth unavailable")
}

func derivePrecisions(major, minor int32) []string {
	if major == 0 {
		return nil
	}
	result := []string{"fp32"}
	if major >= 6 {
		result = append(result, "fp16")
	}
	if major >= 8 {
		result = append(result, "bf16", "int8", "int4")
	}
	return result
}
