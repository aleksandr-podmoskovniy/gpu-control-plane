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

// +k8s:deepcopy-gen=package
// +groupName=gpu.deckhouse.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "gpu.deckhouse.io", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		GroupVersion,
		&GPUDevice{},
		&GPUDeviceList{},
		&GPUNodeState{},
		&GPUNodeStateList{},
		&GPUPool{},
		&GPUPoolList{},
		&ClusterGPUPool{},
		&ClusterGPUPoolList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// +kubebuilder:validation:Enum=Discovered;Validating;Ready;PendingAssignment;Assigned;Reserved;InUse;Faulted
type GPUDeviceState string

const (
	GPUDeviceStateDiscovered        GPUDeviceState = "Discovered"
	GPUDeviceStateValidating        GPUDeviceState = "Validating"
	GPUDeviceStateReady             GPUDeviceState = "Ready"
	GPUDeviceStatePendingAssignment GPUDeviceState = "PendingAssignment"
	GPUDeviceStateAssigned          GPUDeviceState = "Assigned"
	GPUDeviceStateReserved          GPUDeviceState = "Reserved"
	GPUDeviceStateInUse             GPUDeviceState = "InUse"
	GPUDeviceStateFaulted           GPUDeviceState = "Faulted"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=gpudevices,scope=Cluster,shortName=gdevice;gpudev,categories=deckhouse;gpu
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Managed",type=boolean,JSONPath=`.status.managed`
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.status.poolRef.name`
// GPUDevice describes a single physical NVIDIA GPU discovered in the cluster.
type GPUDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds configuration knobs for the device (reserved for future use).
	Spec GPUDeviceSpec `json:"spec,omitempty"`
	// Status contains the observed state of the device collected by the controllers.
	Status GPUDeviceStatus `json:"status,omitempty"`
}

// GPUDeviceSpec is reserved for future configuration options.
type GPUDeviceSpec struct{}

type GPUDeviceStatus struct {
	// NodeName is the name of the Kubernetes node where the device is installed.
	NodeName string `json:"nodeName,omitempty"`
	// InventoryID is a stable identifier for the device (node + PCI address).
	InventoryID string `json:"inventoryID,omitempty"`
	// Managed indicates whether the device is currently managed by Deckhouse controllers.
	Managed bool `json:"managed,omitempty"`
	// State reflects current lifecycle state of the device (unassigned, reserved, assigned, faulted).
	State GPUDeviceState `json:"state,omitempty"`
	// AutoAttach signals that newly detected workloads may be attached automatically without manual approval.
	AutoAttach bool `json:"autoAttach,omitempty"`
	// PoolRef points to the GPUPool that currently owns the device capacity.
	PoolRef *GPUPoolReference `json:"poolRef,omitempty"`
	// Hardware stores static hardware characteristics exported by inventory.
	Hardware GPUDeviceHardware `json:"hardware,omitempty"`
	// Conditions list high-level conditions maintained by controllers (ReadyForPooling, ManagedDisabled, etc.).
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type GPUPoolReference struct {
	// Name is the GPUPool name referencing this device.
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the referenced GPUPool.
	// Empty for ClusterGPUPool.
	Namespace string `json:"namespace,omitempty"`
}

type GPUDeviceHardware struct {
	// UUID is the GPU UUID reported by NVML/DCGM.
	UUID string `json:"uuid,omitempty"`
	// Product is a human readable GPU model as reported by the driver (e.g. NVIDIA A100-PCIE-40GB).
	Product string `json:"product,omitempty"`
	// PCI contains vendor/device/class identifiers describing the PCI function.
	PCI PCIAddress `json:"pci,omitempty"`
	// MIG describes Multi-Instance GPU capabilities and available profiles.
	MIG GPUMIGConfig `json:"mig,omitempty"`
}

type PCIAddress struct {
	// Vendor is the PCI vendor id in hexadecimal (e.g. 10de).
	Vendor string `json:"vendor,omitempty"`
	// Device is the PCI device id in hexadecimal.
	Device string `json:"device,omitempty"`
	// Class is the PCI class code in hexadecimal.
	Class string `json:"class,omitempty"`
	// Address is the full PCI address (domain:bus:slot.func).
	Address string `json:"address,omitempty"`
}

type GPUMIGConfig struct {
	// Capable indicates whether MIG (Multi-Instance GPU) is supported by the device.
	Capable bool `json:"capable,omitempty"`
	// Strategy reflects current MIG strategy detected on the node (None, Single, Mixed).
	Strategy GPUMIGStrategy `json:"strategy,omitempty"`
	// ProfilesSupported enumerates MIG profiles that can be provisioned on this device.
	ProfilesSupported []string `json:"profilesSupported,omitempty"`
	// Types lists capacity counters for each MIG profile provisioned on the device.
	Types []GPUMIGTypeCapacity `json:"types,omitempty"`
}

// +kubebuilder:validation:Enum=None;Single;Mixed
type GPUMIGStrategy string

const (
	GPUMIGStrategyNone   GPUMIGStrategy = "None"
	GPUMIGStrategySingle GPUMIGStrategy = "Single"
	GPUMIGStrategyMixed  GPUMIGStrategy = "Mixed"
)

type GPUMIGTypeCapacity struct {
	// Name is the MIG profile name (for example, 1g.10gb).
	Name string `json:"name,omitempty"`
	// Count represents the number of profiles of this type currently configured.
	Count int32 `json:"count,omitempty"`
}

// +kubebuilder:object:root=true
// GPUDeviceList holds a list of GPUDevice objects.
type GPUDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUDevice `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=gpunodestates,scope=Cluster,categories=deckhouse;gpu
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="ReadyForPooling",type=string,JSONPath=`.status.conditions[?(@.type=="ReadyForPooling")].status`
// GPUNodeState aggregates GPU-related state for a Kubernetes node.
type GPUNodeState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines which node the state belongs to.
	Spec GPUNodeStateSpec `json:"spec,omitempty"`
	// Status surfaces aggregated readiness and alerting via conditions.
	Status GPUNodeStateStatus `json:"status,omitempty"`
}

type GPUNodeStateSpec struct {
	// NodeName is the Kubernetes node name the state describes.
	NodeName string `json:"nodeName,omitempty"`
}

type GPUNodeStateStatus struct {
	// Conditions surfaces aggregated readiness/alerting conditions for the node.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// GPUNodeStateList holds a list of GPUNodeState objects.
type GPUNodeStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUNodeState `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// GPUPool defines a logical pool of GPU capacity exposed to workloads.
type GPUPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec declares desired rules for selecting and slicing devices.
	Spec GPUPoolSpec `json:"spec,omitempty"`
	// Status reports aggregated usage, candidates and conditions for the pool.
	Status GPUPoolStatus `json:"status,omitempty"`
}

type GPUPoolSpec struct {
	// Provider selects GPU vendor implementation (only "Nvidia" is supported for now).
	// +kubebuilder:validation:Enum=Nvidia
	// +kubebuilder:default:="Nvidia"
	Provider string `json:"provider,omitempty"`
	// Backend chooses integration backend (device-plugin or DRA).
	// +kubebuilder:validation:Enum=DevicePlugin;DRA
	// +kubebuilder:default:="DevicePlugin"
	Backend string `json:"backend,omitempty"`
	// Resource defines the resource unit exposed to workloads. Resource name is derived from pool name.
	Resource GPUPoolResourceSpec `json:"resource"`
	// NodeSelector limits the pool to specific nodes.
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// DeviceSelector filters devices that may join the pool.
	DeviceSelector *GPUPoolDeviceSelector `json:"deviceSelector,omitempty"`
	// DeviceAssignment controls manual vs automatic assignment flows.
	DeviceAssignment GPUPoolAssignmentSpec `json:"deviceAssignment,omitempty"`
	// Scheduling configures topology spreading, taints and other scheduling hints.
	Scheduling GPUPoolSchedulingSpec `json:"scheduling,omitempty"`
}

type GPUPoolResourceSpec struct {
	// Unit describes the resource unit (Card or MIG).
	// +kubebuilder:validation:Enum=Card;MIG
	Unit string `json:"unit"`
	// MIGProfile specifies the MIG profile when Unit=MIG (single profile shortcut).
	MIGProfile string `json:"migProfile,omitempty"`
	// MaxDevicesPerNode caps number of devices contributed per node.
	MaxDevicesPerNode *int32 `json:"maxDevicesPerNode,omitempty"`
	// SlicesPerUnit configures oversubscription per base unit (card or MIG partition).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	// +kubebuilder:default:=1
	SlicesPerUnit int32 `json:"slicesPerUnit,omitempty"`
}

type GPUPoolDeviceSelector struct {
	// Include defines positive selection rules for devices.
	Include GPUPoolSelectorRules `json:"include,omitempty"`
	// Exclude defines negative selection rules that remove devices from the pool.
	Exclude GPUPoolSelectorRules `json:"exclude,omitempty"`
}

type GPUPoolSelectorRules struct {
	// InventoryIDs matches specific devices by inventory identifier.
	InventoryIDs []string `json:"inventoryIDs,omitempty"`
	// Products matches devices by reported hardware product name (exact match).
	Products []string `json:"products,omitempty"`
	// PCIVendors matches devices by PCI vendor identifier (hex without 0x, e.g. 10de).
	PCIVendors []string `json:"pciVendors,omitempty"`
	// PCIDevices matches devices by PCI device identifier (hex without 0x, e.g. 20b0).
	PCIDevices []string `json:"pciDevices,omitempty"`
	// MIGCapable restricts selection to devices that are (or are not) MIG capable.
	MIGCapable *bool `json:"migCapable,omitempty"`
	// MIGProfiles matches devices that support at least one of the listed MIG profiles.
	MIGProfiles []string `json:"migProfiles,omitempty"`
}

type GPUPoolAssignmentSpec struct {
	// RequireAnnotation forces manual approval before attaching devices to the pool.
	RequireAnnotation bool `json:"requireAnnotation,omitempty"`
	// AutoApproveSelector lists nodes/devices that may be auto-approved.
	AutoApproveSelector *metav1.LabelSelector `json:"autoApproveSelector,omitempty"`
}

// +kubebuilder:validation:Enum=BinPack;Spread
type GPUPoolSchedulingStrategy string

const (
	GPUPoolSchedulingBinPack GPUPoolSchedulingStrategy = "BinPack"
	GPUPoolSchedulingSpread  GPUPoolSchedulingStrategy = "Spread"
)

type GPUPoolSchedulingSpec struct {
	// Strategy controls balancing strategy among nodes (BinPack or Spread).
	Strategy GPUPoolSchedulingStrategy `json:"strategy,omitempty"`
	// TopologyKey configures topology spreading key when strategy=Spread.
	TopologyKey string `json:"topologyKey,omitempty"`
	// TaintsEnabled controls whether per-pool taints/tolerations are applied.
	TaintsEnabled *bool `json:"taintsEnabled,omitempty"`
	// Taints enumerates taints applied to nodes that host workloads from this pool.
	Taints []GPUPoolTaintSpec `json:"taints,omitempty"`
}

type GPUPoolTaintSpec struct {
	// Key is the taint key applied to nodes hosting the pool.
	Key string `json:"key"`
	// Value is the taint value.
	Value string `json:"value,omitempty"`
	// Effect is the Kubernetes taint effect.
	Effect string `json:"effect,omitempty"`
}

type GPUPoolStatus struct {
	// Capacity summarises total capacity inside the pool.
	Capacity GPUPoolCapacityStatus `json:"capacity,omitempty"`
	// Conditions surfaces pool-level status conditions.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clustergpupools,scope=Cluster,shortName=cgpupool;cgpu,categories=deckhouse;gpu
// ClusterGPUPool defines a cluster-wide pool of GPU capacity exposed to workloads.
type ClusterGPUPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec declares desired rules for selecting and slicing devices.
	Spec GPUPoolSpec `json:"spec,omitempty"`
	// Status reports aggregated usage, candidates and conditions for the pool.
	Status GPUPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// ClusterGPUPoolList holds a list of ClusterGPUPool objects.
type ClusterGPUPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterGPUPool `json:"items"`
}

type GPUPoolCapacityStatus struct {
	// Total is total pool capacity expressed in declared units.
	Total int32 `json:"total,omitempty"`
}

// +kubebuilder:object:root=true
// GPUPoolList holds a list of GPUPool objects.
type GPUPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUPool `json:"items"`
}
