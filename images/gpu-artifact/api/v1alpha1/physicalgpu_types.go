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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PhysicalGPUSpec defines the desired state of PhysicalGPU.
// v0: desired state is intentionally empty.
type PhysicalGPUSpec struct{}

// PhysicalGPUStatus defines the observed state of PhysicalGPU.
type PhysicalGPUStatus struct {
	// NodeInfo contains node identification and OS/kernel details.
	NodeInfo *NodeInfo `json:"nodeInfo,omitempty"`
	// PCIInfo holds PCI identification details for the device.
	PCIInfo *PCIInfo `json:"pciInfo,omitempty"`
	// Capabilities is a snapshot of device capabilities.
	Capabilities *GPUCapabilities `json:"capabilities,omitempty"`
	// Conditions represent the health and readiness of the GPU.
	// +listType=atomic
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// CurrentState reflects current driver binding and mode details.
	CurrentState *GPUCurrentState `json:"currentState,omitempty"`
}

// NodeInfo reports basic OS/kernel information and bare-metal detection.
type NodeInfo struct {
	// NodeName is the Kubernetes node name where the GPU is detected.
	NodeName string `json:"nodeName,omitempty"`
	// OS describes the operating system identification.
	OS *OSInfo `json:"os,omitempty"`
	// KernelRelease is the kernel release string (uname -r).
	KernelRelease string `json:"kernelRelease,omitempty"`
	// BareMetal indicates whether the node is detected as bare metal.
	BareMetal *bool `json:"bareMetal,omitempty"`
}

// OSInfo contains minimal OS release identifiers.
type OSInfo struct {
	// ID is the OS identifier (for example "ubuntu").
	ID string `json:"id,omitempty"`
	// Version is the OS version identifier (for example "22.04").
	Version string `json:"version,omitempty"`
	// Name is the human-readable OS name.
	Name string `json:"name,omitempty"`
}

// PCIInfo holds PCI identity details.
type PCIInfo struct {
	// Address is the PCI address, for example "0000:02:00.0".
	Address string `json:"address,omitempty"`
	// Class describes the PCI class.
	Class *PCIClassInfo `json:"class,omitempty"`
	// Vendor describes the PCI vendor.
	Vendor *PCIVendorInfo `json:"vendor,omitempty"`
	// Device describes the PCI device.
	Device *PCIDeviceInfo `json:"device,omitempty"`
}

// PCIClassInfo describes PCI class details.
type PCIClassInfo struct {
	// Code is the PCI class code (base+subclass), for example "0302".
	Code string `json:"code,omitempty"`
	// Name is the class name (for example "3D controller").
	Name string `json:"name,omitempty"`
}

// PCIVendorInfo describes PCI vendor details.
type PCIVendorInfo struct {
	// ID is the PCI vendor ID, for example "10de".
	ID string `json:"id,omitempty"`
	// Name is the vendor name (for example "NVIDIA Corporation").
	Name string `json:"name,omitempty"`
}

// PCIDeviceInfo describes PCI device details.
type PCIDeviceInfo struct {
	// ID is the PCI device ID, for example "20b7".
	ID string `json:"id,omitempty"`
	// Name is the device name (for example "GA100GL [A30 PCIe]").
	Name string `json:"name,omitempty"`
}

// GPUCapabilities describes the allocatable capabilities of the GPU.
type GPUCapabilities struct {
	// ProductName is the GPU product name (for example "NVIDIA A30").
	ProductName string `json:"productName,omitempty"`
	// MemoryMiB is the total framebuffer memory in MiB.
	MemoryMiB *int64 `json:"memoryMiB,omitempty"`
	// Vendor holds vendor-specific capabilities.
	Vendor *GPUVendorCapabilities `json:"vendor,omitempty"`
}

// GPUVendorCapabilities contains vendor-specific capability snapshots.
type GPUVendorCapabilities struct {
	// Nvidia holds NVIDIA-specific capabilities.
	Nvidia *NvidiaCapabilities `json:"nvidia,omitempty"`
}

// NvidiaCapabilities holds NVIDIA-specific capability data.
type NvidiaCapabilities struct {
	// ComputeCap is the CUDA compute capability string (for example "8.0").
	ComputeCap string `json:"computeCap,omitempty"`
	// ProductArchitecture is the NVIDIA architecture name (for example "Ampere").
	ProductArchitecture string `json:"productArchitecture,omitempty"`
	// BoardPartNumber is the NVIDIA board part number.
	BoardPartNumber string `json:"boardPartNumber,omitempty"`
	// ComputeTypes lists supported compute types.
	// +listType=atomic
	ComputeTypes []string `json:"computeTypes,omitempty"`
	// MIGSupported indicates whether MIG is supported.
	MIGSupported *bool `json:"migSupported,omitempty"`
	// MIG contains MIG capabilities (only when MIGSupported is true).
	MIG *NvidiaMIGCapabilities `json:"mig,omitempty"`
}

// NvidiaMIGCapabilities describes MIG support and profiles.
type NvidiaMIGCapabilities struct {
	// TotalSlices is the total number of MIG slices on the GPU.
	TotalSlices int32 `json:"totalSlices,omitempty"`
	// Profiles lists supported MIG profiles.
	// +listType=atomic
	Profiles []NvidiaMIGProfile `json:"profiles,omitempty"`
}

// NvidiaMIGProfile describes a supported MIG profile.
type NvidiaMIGProfile struct {
	// ProfileID is the numeric profile ID.
	ProfileID int32 `json:"profileID,omitempty"`
	// Name is the profile name, for example "2g.12gb".
	Name string `json:"name,omitempty"`
	// MemoryMiB is the framebuffer size for the profile.
	MemoryMiB int32 `json:"memoryMiB,omitempty"`
	// SliceCount is the number of slices consumed by the profile.
	SliceCount int32 `json:"sliceCount,omitempty"`
	// MaxInstances is the maximum number of instances for this profile.
	MaxInstances int32 `json:"maxInstances,omitempty"`
}

// GPUCurrentState describes current driver binding and mode details.
type GPUCurrentState struct {
	// DriverType is the current driver binding (Nvidia/VFIO/ROCm).
	DriverType DriverType `json:"driverType,omitempty"`
	// Nvidia holds NVIDIA-specific current state (when driverType=Nvidia).
	Nvidia *NvidiaCurrentState `json:"nvidia,omitempty"`
}

// NvidiaCurrentState holds NVIDIA-specific current state details.
type NvidiaCurrentState struct {
	// GPUUUID is the NVIDIA GPU UUID.
	GPUUUID string `json:"gpuUUID,omitempty"`
	// DriverVersion is the NVIDIA driver version string.
	DriverVersion string `json:"driverVersion,omitempty"`
	// CUDAVersion is the reported CUDA version string.
	CUDAVersion string `json:"cudaVersion,omitempty"`
	// MIG describes the current MIG mode (when supported).
	MIG *NvidiaMIGState `json:"mig,omitempty"`
}

// NvidiaMIGState contains current MIG mode info.
type NvidiaMIGState struct {
	// Mode is the current MIG mode.
	Mode MIGModeState `json:"mode,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pgpu,categories=deckhouse;gpu
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeInfo.nodeName`
// +kubebuilder:printcolumn:name="Vendor",type=string,JSONPath=`.metadata.labels.gpu\.deckhouse\.io/vendor`

// PhysicalGPU is the Schema for the physicalgpus API.
type PhysicalGPU struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PhysicalGPUSpec   `json:"spec,omitempty"`
	Status PhysicalGPUStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PhysicalGPUList contains a list of PhysicalGPU.
type PhysicalGPUList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhysicalGPU `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PhysicalGPU{}, &PhysicalGPUList{})
}
