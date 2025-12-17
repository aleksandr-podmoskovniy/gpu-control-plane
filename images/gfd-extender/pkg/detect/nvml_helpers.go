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

import "fmt"

func decodeNVMLPciDeviceID(pciDeviceID uint32) (vendor, device string) {
	// NVML encodes the combined PCI ID as: (deviceID << 16) | vendorID.
	vendor = fmt.Sprintf("%04x", pciDeviceID&0xffff)
	device = fmt.Sprintf("%04x", pciDeviceID>>16)
	return vendor, device
}

func migInfoFromGetMigMode(supported bool, migMode int) (capable bool, mode string) {
	if !supported {
		return false, ""
	}

	switch migMode {
	case 0:
		return true, "disabled"
	case 1:
		return true, "enabled"
	default:
		return true, fmt.Sprintf("%d", migMode)
	}
}
