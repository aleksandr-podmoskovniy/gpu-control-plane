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

package allocatable

const (
	AttrVendor     = "gpu.deckhouse.io/vendor"
	AttrDeviceType = "gpu.deckhouse.io/deviceType"
	AttrDevice     = "gpu.deckhouse.io/device"
	AttrPCIAddress = "gpu.deckhouse.io/pciAddress"
	AttrGPUUUID    = "gpu.deckhouse.io/gpuUUID"
	AttrDriverVer  = "gpu.deckhouse.io/driverVersion"
	AttrCCMajor    = "gpu.deckhouse.io/ccMajor"
	AttrCCMinor    = "gpu.deckhouse.io/ccMinor"
	AttrMigProfile = "gpu.deckhouse.io/migProfile"
	AttrMigUUID    = "gpu.deckhouse.io/migUUID"
	AttrMpsPipeDir = "gpu.deckhouse.io/mpsPipeDir"
	AttrMpsShmDir  = "gpu.deckhouse.io/mpsShmDir"
	AttrMpsLogDir  = "gpu.deckhouse.io/mpsLogDir"

	CapMemory       = "memory"
	CapSharePercent = "sharePercent"
)
