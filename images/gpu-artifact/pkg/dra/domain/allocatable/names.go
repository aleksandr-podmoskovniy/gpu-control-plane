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

import "strings"

// SanitizeDNSLabel normalizes a string to a DNS label.
func SanitizeDNSLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "gpu"
	}

	var b strings.Builder
	b.Grow(len(value))
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	normalized := strings.Trim(b.String(), "-")
	if normalized == "" {
		return "gpu"
	}
	if len(normalized) <= 63 {
		return normalized
	}
	normalized = normalized[:63]
	return strings.Trim(normalized, "-")
}

// CounterSetNameForPCI builds a stable counterSet name for a PCI address.
func CounterSetNameForPCI(pci string) string {
	pci = strings.TrimSpace(pci)
	if pci == "" {
		return ""
	}
	return SanitizeDNSLabel("pgpu-" + pci)
}
