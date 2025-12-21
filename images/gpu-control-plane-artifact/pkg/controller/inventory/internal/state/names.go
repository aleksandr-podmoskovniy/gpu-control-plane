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

import "strings"

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

