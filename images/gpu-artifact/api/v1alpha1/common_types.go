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

package v1alpha1

// +kubebuilder:validation:Enum=Nvidia;Amd;Intel
// VendorName identifies the GPU vendor.
type VendorName string

const (
	VendorNvidia VendorName = "Nvidia"
	VendorAMD    VendorName = "Amd"
	VendorIntel  VendorName = "Intel"
)

// +kubebuilder:validation:Enum=Physical;MIG
// DeviceType defines how a GPU is allocated to a workload.
type DeviceType string

const (
	DeviceTypePhysical DeviceType = "Physical"
	DeviceTypeMIG      DeviceType = "MIG"
)

// +kubebuilder:validation:Enum=Exclusive;TimeSlicing;MPS
// GPUSharingStrategy defines the collaborative sharing strategy.
type GPUSharingStrategy string

const (
	GPUSharingExclusive   GPUSharingStrategy = "Exclusive"
	GPUSharingTimeSlicing GPUSharingStrategy = "TimeSlicing"
	GPUSharingMPS         GPUSharingStrategy = "MPS"
)

// +kubebuilder:validation:Enum=Default;Short;Medium;Long
// TimeSlicingInterval defines the time-slicing interval hint.
type TimeSlicingInterval string

const (
	TimeSlicingDefault TimeSlicingInterval = "Default"
	TimeSlicingShort   TimeSlicingInterval = "Short"
	TimeSlicingMedium  TimeSlicingInterval = "Medium"
	TimeSlicingLong    TimeSlicingInterval = "Long"
)

// +kubebuilder:validation:Enum=Nvidia;VFIO;ROCm
// DriverType describes the current driver binding for a GPU.
type DriverType string

const (
	DriverTypeNvidia DriverType = "Nvidia"
	DriverTypeVFIO   DriverType = "VFIO"
	DriverTypeROCm   DriverType = "ROCm"
)

// +kubebuilder:validation:Enum=Enabled;Disabled;NotAvailable;Unknown
// MIGModeState is the current MIG mode on a GPU.
type MIGModeState string

const (
	MIGModeEnabled  MIGModeState = "Enabled"
	MIGModeDisabled MIGModeState = "Disabled"
	MIGModeNA       MIGModeState = "NotAvailable"
	MIGModeUnknown  MIGModeState = "Unknown"
)
