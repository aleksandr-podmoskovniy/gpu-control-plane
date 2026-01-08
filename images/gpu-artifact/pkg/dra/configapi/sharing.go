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

package configapi

import "k8s.io/apimachinery/pkg/api/resource"

// These constants represent the different Sharing strategies.
const (
	TimeSlicingStrategy = "TimeSlicing"
	MpsStrategy         = "MPS"
)

// These constants represent the different TimeSlicing configurations.
const (
	DefaultTimeSlice TimeSliceInterval = "Default"
	ShortTimeSlice   TimeSliceInterval = "Short"
	MediumTimeSlice  TimeSliceInterval = "Medium"
	LongTimeSlice    TimeSliceInterval = "Long"
)

// Sharing provides methods to check the sharing strategy and its configuration.
// +k8s:deepcopy-gen=false
type Sharing interface {
	IsTimeSlicing() bool
	IsMps() bool
	GetTimeSlicingConfig() (*TimeSlicingConfig, error)
	GetMpsConfig() (*MpsConfig, error)
}

// GpuSharingStrategy encodes the valid Sharing strategies as a string.
type GpuSharingStrategy string

// TimeSliceInterval encodes the valid timeslice duration as a string.
type TimeSliceInterval string

// MpsPerDevicePinnedMemoryLimit holds the limits across multiple devices.
type MpsPerDevicePinnedMemoryLimit map[string]resource.Quantity

// GpuSharing holds the current sharing strategy for GPUs and its settings.
type GpuSharing struct {
	Strategy          GpuSharingStrategy `json:"strategy"`
	TimeSlicingConfig *TimeSlicingConfig `json:"timeSlicingConfig,omitempty"`
	MpsConfig         *MpsConfig         `json:"mpsConfig,omitempty"`
}

// MigDeviceSharing holds the current sharing strategy for MIG Devices and its settings.
type MigDeviceSharing struct {
	Strategy  GpuSharingStrategy `json:"strategy"`
	MpsConfig *MpsConfig         `json:"mpsConfig,omitempty"`
}

// TimeSlicingConfig provides the settings for CUDA time-slicing.
type TimeSlicingConfig struct {
	Interval *TimeSliceInterval `json:"interval,omitempty"`
}

// MpsConfig provides configuration for an MPS control daemon.
type MpsConfig struct {
	DefaultActiveThreadPercentage *int `json:"defaultActiveThreadPercentage,omitempty"`
	// DefaultPinnedDeviceMemoryLimit represents the pinned memory limit to be applied for all devices.
	DefaultPinnedDeviceMemoryLimit *resource.Quantity `json:"defaultPinnedDeviceMemoryLimit,omitempty"`
	// DefaultPerDevicePinnedMemoryLimit represents the pinned memory limit per device.
	DefaultPerDevicePinnedMemoryLimit MpsPerDevicePinnedMemoryLimit `json:"defaultPerDevicePinnedMemoryLimit,omitempty"`
}
