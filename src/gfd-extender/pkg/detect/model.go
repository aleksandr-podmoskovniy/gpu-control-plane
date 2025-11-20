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

// Info represents the subset of NVML data exposed to controller.
type Info struct {
	Index                       int           `json:"index"`
	UUID                        string        `json:"uuid"`
	Name                        string        `json:"name"`
	Product                     string        `json:"product"`
	MemoryInfo                  MemoryInfo    `json:"memoryInfo"`
	MemoryInfoV2                MemoryInfoV2  `json:"memoryInfoV2"`
	PowerUsage                  uint32        `json:"powerUsage"`
	PowerState                  PState        `json:"powerState"`
	PowerManagementDefaultLimit uint32        `json:"powerManagementDefaultLimit"`
	InformImageVersion          string        `json:"informImageVersion"`
	DriverVersion               string        `json:"systemGetDriverVersion"`
	CUDADriverVersion           int           `json:"systemGetCudaDriverVersion"`
	GraphicsRunningProcesses    []ProcessInfo `json:"graphicsRunningProcesses"`
	Utilization                 Utilization   `json:"utilization"`
	MemoryMiB                   int32         `json:"memoryMiB"`
	ComputeMajor                int32         `json:"computeMajor"`
	ComputeMinor                int32         `json:"computeMinor"`
	NUMANode                    *int32        `json:"numaNode,omitempty"`
	SMCount                     *int32        `json:"smCount,omitempty"`
	MemoryBandwidthMiB          *int32        `json:"memoryBandwidthMiB,omitempty"`
	PCI                         PCIInfo       `json:"pci"`
	PCIE                        PCIELink      `json:"pcie"`
	Board                       string        `json:"board"`
	Family                      string        `json:"family"`
	Serial                      string        `json:"serial"`
	DisplayMode                 string        `json:"displayMode"`
	MIG                         MIGInfo       `json:"mig"`
	// Partial indicates that some fields failed to collect; details are in Warnings.
	Partial  bool     `json:"partial,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type PCIInfo struct {
	Address   string `json:"address"`
	Vendor    string `json:"vendor"`
	Device    string `json:"device"`
	Class     string `json:"class"`
	Subsystem string `json:"subsystem,omitempty"`
}

type PCIELink struct {
	Generation *int32 `json:"generation,omitempty"`
	Width      *int32 `json:"width,omitempty"`
}

type MIGInfo struct {
	Capable           bool     `json:"capable"`
	Mode              string   `json:"mode,omitempty"`
	ProfilesSupported []string `json:"profilesSupported,omitempty"`
}

type MemoryInfo struct {
	Total uint64 `json:"Total"`
	Free  uint64 `json:"Free"`
	Used  uint64 `json:"Used"`
}

type MemoryInfoV2 struct {
	Version  uint32 `json:"Version"`
	Total    uint64 `json:"Total"`
	Reserved uint64 `json:"Reserved"`
	Free     uint64 `json:"Free"`
	Used     uint64 `json:"Used"`
}

type ProcessInfo struct {
	Pid               uint32 `json:"Pid"`
	UsedGpuMemory     uint64 `json:"UsedGpuMemory"`
	GpuInstanceId     uint32 `json:"GpuInstanceId"`
	ComputeInstanceId uint32 `json:"ComputeInstanceId"`
}

type Utilization struct {
	GPU    uint32 `json:"Gpu"`
	Memory uint32 `json:"Memory"`
}

type PState uint32
