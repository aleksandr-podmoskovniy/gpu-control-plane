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

package service

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func formatCudaVersion(raw int) string {
	if raw <= 0 {
		return ""
	}
	major := raw / 1000
	minor := (raw % 1000) / 10
	return fmt.Sprintf("%d.%d", major, minor)
}

func architectureName(arch nvml.DeviceArchitecture) string {
	switch arch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "Kepler"
	case nvml.DEVICE_ARCH_MAXWELL:
		return "Maxwell"
	case nvml.DEVICE_ARCH_PASCAL:
		return "Pascal"
	case nvml.DEVICE_ARCH_VOLTA:
		return "Volta"
	case nvml.DEVICE_ARCH_TURING:
		return "Turing"
	case nvml.DEVICE_ARCH_AMPERE:
		return "Ampere"
	case nvml.DEVICE_ARCH_ADA:
		return "Ada"
	case nvml.DEVICE_ARCH_HOPPER:
		return "Hopper"
	case nvml.DEVICE_ARCH_BLACKWELL:
		return "Blackwell"
	default:
		return ""
	}
}

func migModeFromNVML(mode int) gpuv1alpha1.MIGModeState {
	switch mode {
	case nvml.DEVICE_MIG_ENABLE:
		return gpuv1alpha1.MIGModeEnabled
	case nvml.DEVICE_MIG_DISABLE:
		return gpuv1alpha1.MIGModeDisabled
	default:
		return gpuv1alpha1.MIGModeUnknown
	}
}

func nvmlName(raw []uint8) string {
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i])
		}
	}
	return string(raw)
}

func nvmlString(call func() (string, nvml.Return)) string {
	value, ret := call()
	if ret != nvml.SUCCESS {
		return ""
	}
	return value
}

func nvmlPowerLimit(call func() (uint32, nvml.Return)) *int64 {
	value, ret := call()
	if ret != nvml.SUCCESS {
		return nil
	}
	limit := milliwattsToWatts(value)
	return &limit
}

func milliwattsToWatts(value uint32) int64 {
	return int64(value) / 1000
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
