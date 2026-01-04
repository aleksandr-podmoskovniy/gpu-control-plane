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

package driver

import "strings"

const (
	// VfioAnnotationKey enables VFIO mode for an exclusive Physical allocation.
	VfioAnnotationKey = "gpu.deckhouse.io/vfio"
)

// VFIORequested reports whether a pod annotation requests VFIO mode.
func VFIORequested(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	raw := strings.TrimSpace(annotations[VfioAnnotationKey])
	if raw == "" {
		return false
	}
	switch strings.ToLower(raw) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}
