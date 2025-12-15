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

import "strings"

// canonicalizePCIAddress normalizes PCI bus address representation.
//
// NVML commonly returns addresses in form "00000000:65:00.0" (8-hex-digit domain),
// while Linux tools typically render "0000:65:00.0" (4-hex-digit domain).
// We keep the canonical 4-hex-digit domain when the input matches a known format.
func canonicalizePCIAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	addr = strings.ToLower(addr)

	parts := strings.Split(addr, ":")
	if len(parts) != 3 {
		return addr
	}

	domain := parts[0]
	bus := parts[1]
	devfn := parts[2]

	devParts := strings.Split(devfn, ".")
	if len(devParts) != 2 {
		return addr
	}
	device := devParts[0]
	function := devParts[1]

	if len(domain) == 8 {
		domain = domain[4:]
	}

	if len(domain) != 4 || len(bus) != 2 || len(device) != 2 || len(function) != 1 {
		return addr
	}
	if !isHex(domain) || !isHex(bus) || !isHex(device) {
		return addr
	}
	if function[0] < '0' || function[0] > '7' {
		return addr
	}

	return domain + ":" + bus + ":" + device + "." + function
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

