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

package handler

import (
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func driverTypeFromName(name string) gpuv1alpha1.DriverType {
	switch strings.ToLower(name) {
	case "nvidia":
		return gpuv1alpha1.DriverTypeNvidia
	case "vfio-pci":
		return gpuv1alpha1.DriverTypeVFIO
	case "amdgpu":
		return gpuv1alpha1.DriverTypeROCm
	default:
		return ""
	}
}
