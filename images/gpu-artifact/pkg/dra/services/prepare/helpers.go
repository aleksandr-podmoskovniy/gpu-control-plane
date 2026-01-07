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

package prepare

import (
	"strings"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func attrString(attrs map[string]allocatable.AttributeValue, key string) string {
	if attrs == nil {
		return ""
	}
	val, ok := attrs[key]
	if !ok || val.String == nil {
		return ""
	}
	return strings.TrimSpace(*val.String)
}

func cloneAttributes(attrs map[string]allocatable.AttributeValue) map[string]allocatable.AttributeValue {
	if len(attrs) == 0 {
		return map[string]allocatable.AttributeValue{}
	}
	out := make(map[string]allocatable.AttributeValue, len(attrs))
	for key, val := range attrs {
		out[key] = val
	}
	return out
}

func isMigDevice(deviceType string) bool {
	return strings.EqualFold(deviceType, "mig")
}

func isPhysicalDevice(deviceType string) bool {
	return strings.EqualFold(deviceType, "physical")
}
