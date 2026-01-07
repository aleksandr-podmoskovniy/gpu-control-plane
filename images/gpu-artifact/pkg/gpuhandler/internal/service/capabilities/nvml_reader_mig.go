//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package capabilities

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func readMigMode(dev NVMLDevice) *gpuv1alpha1.NvidiaMIGState {
	current, _, ret := dev.GetMigMode()
	if ret != nvml.SUCCESS {
		return nil
	}
	return &gpuv1alpha1.NvidiaMIGState{Mode: migModeFromNVML(current)}
}

func readMigCapabilities(dev NVMLDevice) (*bool, *gpuv1alpha1.NvidiaMIGCapabilities) {
	_, _, ret := dev.GetMigMode()
	switch ret {
	case nvml.SUCCESS:
		supported := true
		return boolPtr(supported), buildMigProfiles(dev)
	case nvml.ERROR_NOT_SUPPORTED:
		supported := false
		return boolPtr(supported), nil
	default:
		return nil, nil
	}
}
