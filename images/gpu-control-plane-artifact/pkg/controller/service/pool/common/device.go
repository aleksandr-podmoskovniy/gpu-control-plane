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

package common

import (
	"strings"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func IsDeviceIgnored(dev *v1alpha1.GPUDevice) bool {
	if dev == nil {
		return false
	}
	return strings.EqualFold(dev.Labels[DeviceIgnoreKey], "true")
}

func DeviceNodeName(dev *v1alpha1.GPUDevice) string {
	if dev == nil {
		return ""
	}
	if nodeName := strings.TrimSpace(dev.Status.NodeName); nodeName != "" {
		return nodeName
	}
	return ""
}
