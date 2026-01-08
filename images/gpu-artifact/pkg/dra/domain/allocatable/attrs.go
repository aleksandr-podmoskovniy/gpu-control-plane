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

// AttrString returns a string attribute or empty string.
func AttrString(attrs map[string]AttributeValue, key string) string {
	if attrs == nil {
		return ""
	}
	val, ok := attrs[key]
	if !ok || val.String == nil {
		return ""
	}
	return strings.TrimSpace(*val.String)
}

// IsMigDevice returns true for MIG device type.
func IsMigDevice(deviceType string) bool {
	return strings.EqualFold(deviceType, "mig")
}

// IsPhysicalDevice returns true for physical GPU device type.
func IsPhysicalDevice(deviceType string) bool {
	return strings.EqualFold(deviceType, "physical")
}
