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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GPUClassSpec defines the desired state of GPUClass.
type GPUClassSpec struct {
	// Vendor is the GPU vendor.
	Vendor VendorName `json:"vendor"`
	// DeviceType defines the allocation unit type.
	DeviceType DeviceType `json:"deviceType"`
	// Requirements are optional minimum requirements.
	Requirements *GPUClassRequirements `json:"requirements,omitempty"`
	// Nvidia holds NVIDIA-specific profile requirements.
	Nvidia *GPUClassNvidiaSpec `json:"nvidia,omitempty"`
	// Sharing defines collaborative sharing settings.
	Sharing *GPUSharing `json:"sharing,omitempty"`
	// Selector limits eligible devices by labels.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// Names is an optional allowlist of device identifiers.
	// +listType=atomic
	Names []string `json:"names,omitempty"`
}

// GPUClassRequirements defines minimum hardware requirements.
type GPUClassRequirements struct {
	// MinMemory is the minimum framebuffer size required.
	MinMemory *resource.Quantity `json:"minMemory,omitempty"`
	// MinComputeCapability is a CUDA compute capability string (e.g. "8.0").
	// +kubebuilder:validation:Pattern=^\d+\.\d+$
	MinComputeCapability string `json:"minComputeCapability,omitempty"`
}

// GPUClassNvidiaSpec defines NVIDIA-specific GPU class constraints.
type GPUClassNvidiaSpec struct {
	// MIG profile requirements (for deviceType=MIG).
	MIG *GPUClassMIGSpec `json:"mig,omitempty"`
}

// GPUClassMIGSpec defines a required MIG profile.
type GPUClassMIGSpec struct {
	// Profile is the MIG profile name, for example "2g.12gb".
	Profile string `json:"profile"`
}

// GPUSharing defines collaborative sharing settings.
type GPUSharing struct {
	// Strategy selects the sharing strategy.
	Strategy GPUSharingStrategy `json:"strategy"`
	// TimeSlicing contains time-slicing settings (for strategy=TimeSlicing).
	TimeSlicing *TimeSlicingConfig `json:"timeSlicing,omitempty"`
	// MPS contains MPS settings (for strategy=MPS).
	MPS *MPSConfig `json:"mps,omitempty"`
}

// TimeSlicingConfig defines time-slicing parameters.
type TimeSlicingConfig struct {
	// Interval is a time-slicing interval hint.
	Interval TimeSlicingInterval `json:"interval,omitempty"`
}

// MPSConfig defines MPS parameters.
type MPSConfig struct {
	// DefaultActiveThreadPercentage sets the default MPS active thread percentage (1..100).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	DefaultActiveThreadPercentage *int32 `json:"defaultActiveThreadPercentage,omitempty"`
}

// GPUClassStatus defines the observed state of GPUClass.
type GPUClassStatus struct {
	// Valid indicates whether the GPUClass spec is valid.
	Valid *bool `json:"valid,omitempty"`
	// Message provides a human-readable status message.
	Message string `json:"message,omitempty"`
	// Generated holds denormalized data for diagnostics.
	Generated *GPUClassGeneratedStatus `json:"generated,omitempty"`
}

// GPUClassGeneratedStatus contains generated identifiers.
type GPUClassGeneratedStatus struct {
	// DeviceClassName is the generated DeviceClass name.
	DeviceClassName string `json:"deviceClassName,omitempty"`
	// ExtendedResourceName is optional for Kubernetes >= 1.34.
	ExtendedResourceName string `json:"extendedResourceName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=gpuc,categories=deckhouse;gpu
// +kubebuilder:printcolumn:name="Vendor",type=string,JSONPath=`.spec.vendor`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.deviceType`
// +kubebuilder:printcolumn:name="Valid",type=string,JSONPath=`.status.valid`

// GPUClass is the Schema for the gpuclasses API.
type GPUClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GPUClassSpec   `json:"spec,omitempty"`
	Status GPUClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GPUClassList contains a list of GPUClass.
type GPUClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GPUClass{}, &GPUClassList{})
}
