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

import v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"

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
