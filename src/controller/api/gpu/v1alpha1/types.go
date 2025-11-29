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
		&GPUNodeInventory{},
		&GPUNodeInventoryList{},
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
	// Health aggregates latest telemetry (temperatures, ECC counters, vendor metrics).
	Health GPUDeviceHealth `json:"health,omitempty"`
	// Conditions list high-level conditions maintained by controllers (ReadyForPooling, ManagedDisabled, etc.).
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type GPUPoolReference struct {
	// Name is the GPUPool name referencing this device.
	Name string `json:"name,omitempty"`
	// Resource is the fully qualified resource name exposed via device-plugin for this pool.
	Resource string `json:"resource,omitempty"`
}

type GPUDeviceHardware struct {
	// UUID is the GPU UUID reported by NVML/DCGM.
	UUID string `json:"uuid,omitempty"`
	// Product is a human readable GPU model as reported by the driver (e.g. NVIDIA A100-PCIE-40GB).
	Product string `json:"product,omitempty"`
	// PCI contains vendor/device/class identifiers describing the PCI function.
	PCI PCIAddress `json:"pci,omitempty"`
	// NUMANode is the NUMA node id associated with the device, if any.
	NUMANode *int32 `json:"numaNode,omitempty"`
	// PowerLimitMilliWatt is the enforced power limit.
	PowerLimitMilliWatt *int32 `json:"powerLimitMilliWatt,omitempty"`
	// SMCount is the number of streaming multiprocessors.
	SMCount *int32 `json:"smCount,omitempty"`
	// MemoryBandwidthMiB is the memory bandwidth in MiB/s.
	MemoryBandwidthMiB *int32 `json:"memoryBandwidthMiB,omitempty"`
	// PCIE reflects PCIe generation and link width.
	PCIE PCIELink `json:"pcie,omitempty"`
	// Board is the board identifier/model.
	Board string `json:"board,omitempty"`
	// Family is the GPU family name.
	Family string `json:"family,omitempty"`
	// Serial is the GPU serial number if reported.
	Serial string `json:"serial,omitempty"`
	// PState is the current performance state.
	PState string `json:"pstate,omitempty"`
	// DisplayMode indicates whether display is enabled.
	DisplayMode string `json:"displayMode,omitempty"`
	// MemoryMiB is the total memory size of the device in MiB.
	MemoryMiB int32 `json:"memoryMiB,omitempty"`
	// ComputeCapability holds CUDA compute capability reported for the device.
	ComputeCapability *GPUComputeCapability `json:"computeCapability,omitempty"`
	// Precision enumerates supported math precisions (fp32/fp16/bf16/etc.).
	Precision GPUPrecision `json:"precision,omitempty"`
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

// PCIELink captures PCIe link characteristics.
type PCIELink struct {
	// Generation is the PCIe generation (e.g. 3,4,5).
	Generation *int32 `json:"generation,omitempty"`
	// Width is the negotiated link width (e.g. 16).
	Width *int32 `json:"width,omitempty"`
}

type GPUComputeCapability struct {
	// Major is the major version of CUDA compute capability.
	Major int32 `json:"major,omitempty"`
	// Minor is the minor version of CUDA compute capability.
	Minor int32 `json:"minor,omitempty"`
}

type GPUPrecision struct {
	// Supported lists all math precisions supported by the device (e.g. fp32, fp16, bf16).
	Supported []string `json:"supported,omitempty"`
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
	// MemoryMiB is the per-partition memory size in MiB.
	MemoryMiB int32 `json:"memoryMiB,omitempty"`
	// Multiprocessors indicates the number of SMs allocated to this partition.
	Multiprocessors int32 `json:"multiprocessors,omitempty"`
	// Partition details the GPU and Compute instance identifiers associated with the profile.
	Partition GPUMIGPartition `json:"partition,omitempty"`
	// Engines lists the number of acceleration engines exposed to the partition (copy/encoder/decoder/ofa).
	Engines GPUMIGEngines `json:"engines,omitempty"`
}

type GPUMIGPartition struct {
	// GPUInstance is the identifier of the GPU instance allocated for the partition.
	GPUInstance int32 `json:"gpuInstance,omitempty"`
	// ComputeInstance is the identifier of the compute instance allocated for the partition.
	ComputeInstance int32 `json:"computeInstance,omitempty"`
}

type GPUMIGEngines struct {
	// Copy is the number of copy engines assigned to the partition.
	Copy int32 `json:"copy,omitempty"`
	// Encoder is the number of NVENC engines assigned to the partition.
	Encoder int32 `json:"encoder,omitempty"`
	// Decoder is the number of NVDEC engines assigned to the partition.
	Decoder int32 `json:"decoder,omitempty"`
	// OFAs is the number of Optical Flow Accelerators available in the partition.
	OFAs int32 `json:"ofa,omitempty"`
}

type GPUDeviceHealth struct {
	// TemperatureC is the latest reported temperature in Celsius.
	TemperatureC int32 `json:"temperatureC,omitempty"`
	// ECCErrorsTotal is the cumulative count of ECC errors reported by DCGM.
	ECCErrorsTotal int64 `json:"eccErrorsTotal,omitempty"`
	// LastUpdatedTime is when telemetry was last refreshed.
	LastUpdatedTime *metav1.Time `json:"lastUpdated,omitempty"`
	// LastHealthyTime stores the timestamp of the last heartbeat without faults.
	LastHealthyTime *metav1.Time `json:"lastHealthyTime,omitempty"`
	// ConsecutiveHealthy reports the number of sequential healthy heartbeats.
	ConsecutiveHealthy int32 `json:"consecutiveHealthy,omitempty"`
	// LastError contains the latest fault message detected for the device.
	LastError string `json:"lastError,omitempty"`
	// LastErrorReason categorises the latest fault (XID, ECC, etc.).
	LastErrorReason string `json:"lastErrorReason,omitempty"`
	// LastErrorTime records when the latest fault was observed.
	LastErrorTime *metav1.Time `json:"lastErrorTime,omitempty"`
	// Metrics contains vendor specific telemetry key/value pairs exported by controllers.
	Metrics map[string]string `json:"metrics,omitempty"`
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
// +kubebuilder:resource:path=gpunodeinventories,scope=Cluster,shortName=gpunode;gpnode,categories=deckhouse;gpu
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="ReadyForPooling",type=string,JSONPath=`.status.conditions[?(@.type=="ReadyForPooling")].status`
// GPUNodeInventory aggregates GPU-related state for a Kubernetes node.
type GPUNodeInventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines which node the inventory belongs to.
	Spec GPUNodeInventorySpec `json:"spec,omitempty"`
	// Status reflects hardware, driver and pooling state detected on the node.
	Status GPUNodeInventoryStatus `json:"status,omitempty"`
}

type GPUNodeInventorySpec struct {
	// NodeName is the Kubernetes node name the inventory describes.
	NodeName string `json:"nodeName,omitempty"`
}

type GPUNodeInventoryStatus struct {
	// Hardware captures node-level hardware presence flags.
	Hardware GPUNodeHardware `json:"hw,omitempty"`
	// Driver captures driver/toolkit versions and readiness.
	Driver GPUNodeDriver `json:"driver,omitempty"`
	// Devices lists all detected GPUs with mirrored device metadata.
	Devices []GPUNodeDevice `json:"devices,omitempty"`
	// Monitoring indicates health of GFD/DCGM exporters.
	Monitoring GPUNodeMonitoring `json:"monitoring,omitempty"`
	// Bootstrap describes results of bootstrap-controller checks for the node.
	Bootstrap GPUNodeBootstrapStatus `json:"bootstrap,omitempty"`
	// Conditions surfaces aggregated readiness/alerting conditions for the node.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type GPUNodeHardware struct {
	// Present indicates whether any supported GPU devices are present on the node.
	Present bool `json:"present,omitempty"`
}

type GPUNodeDevice struct {
	// InventoryID mirrors GPUDevice.status.inventoryID for quick lookup.
	InventoryID string `json:"inventoryID,omitempty"`
	// UUID is the GPU UUID obtained from NVML/DCGM.
	UUID string `json:"uuid,omitempty"`
	// Product is the GPU model available on this node.
	Product string `json:"product,omitempty"`
	// Family is the GPU family name.
	Family string `json:"family,omitempty"`
	// PCI describes vendor/device/class identifiers for the PCI function.
	PCI PCIAddress `json:"pci,omitempty"`
	// NUMANode is the NUMA node id associated with the device.
	NUMANode *int32 `json:"numaNode,omitempty"`
	// MemoryMiB is device memory capacity in MiB.
	MemoryMiB int32 `json:"memoryMiB,omitempty"`
	// MIG summarises MIG capabilities for this device.
	MIG GPUMIGConfig `json:"mig,omitempty"`
	// ComputeCap is the reported CUDA compute capability.
	ComputeCap *GPUComputeCapability `json:"computeCapability,omitempty"`
	// State mirrors GPUDevice.state for convenience.
	State GPUDeviceState `json:"state,omitempty"`
	// LastError contains the latest fault message detected for the device.
	LastError string `json:"lastError,omitempty"`
	// LastErrorReason categorises the latest fault (XID, ECC, etc.).
	LastErrorReason string `json:"lastErrorReason,omitempty"`
	// LastUpdatedTime records when the latest device data was observed.
	LastUpdatedTime *metav1.Time `json:"lastUpdatedTime,omitempty"`
}

type GPUNodeDriver struct {
	// Version is the NVIDIA driver version detected on the node.
	Version string `json:"version,omitempty"`
	// CUDAVersion is the CUDA runtime version available on the node.
	CUDAVersion string `json:"cudaVersion,omitempty"`
	// ToolkitReady indicates that required CUDA toolkit components are installed.
	ToolkitReady bool `json:"toolkitInstalled,omitempty"`
}

type GPUNodeMonitoring struct {
	// DCGMReady shows whether DCGM exporter is running and responsive.
	DCGMReady bool `json:"dcgmReady,omitempty"`
	// LastHeartbeat records the timestamp of the last successful monitoring heartbeat.
	LastHeartbeat *metav1.Time `json:"lastHeartbeat,omitempty"`
	// ConsecutiveHeartbeats counts sequential successful heartbeats from DCGM exporter.
	ConsecutiveHeartbeats int32 `json:"consecutiveHeartbeats,omitempty"`
	// ConsecutiveFailures counts sequential failed monitoring checks (missing pods/heartbeats).
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
}

// +kubebuilder:validation:Enum=Disabled;Validating;ValidatingFailed;GFD;Monitoring;Ready
// GPUNodeBootstrapPhase enumerates bootstrap phases for a node.
type GPUNodeBootstrapPhase string

const (
	// GPUNodeBootstrapPhaseDisabled indicates that the node is managed-disabled and bootstrap workloads are off.
	GPUNodeBootstrapPhaseDisabled GPUNodeBootstrapPhase = "Disabled"
	// GPUNodeBootstrapPhaseValidating signals that driver/toolkit validation is in progress.
	GPUNodeBootstrapPhaseValidating GPUNodeBootstrapPhase = "Validating"
	// GPUNodeBootstrapPhaseValidatingFailed signals that driver/toolkit validation failed.
	GPUNodeBootstrapPhaseValidatingFailed GPUNodeBootstrapPhase = "ValidatingFailed"
	// GPUNodeBootstrapPhaseGFD indicates that GFD is running and synchronising labels.
	GPUNodeBootstrapPhaseGFD GPUNodeBootstrapPhase = "GFD"
	// GPUNodeBootstrapPhaseMonitoring indicates that DCGM is running but node is not yet ready for pooling.
	GPUNodeBootstrapPhaseMonitoring GPUNodeBootstrapPhase = "Monitoring"
	// GPUNodeBootstrapPhaseReady indicates that the node passed all bootstrap checks.
	GPUNodeBootstrapPhaseReady GPUNodeBootstrapPhase = "Ready"
)

type GPUNodeBootstrapStatus struct {
	// Phase reflects the current bootstrap phase.
	Phase GPUNodeBootstrapPhase `json:"phase,omitempty"`
	// Components stores enablement flags for bootstrap workloads on this node.
	Components map[string]bool `json:"components,omitempty"`
	// GFDReady indicates that GPU Feature Discovery DaemonSet is successfully running.
	GFDReady bool `json:"gfdReady,omitempty"`
	// ToolkitReady signals that toolkit preparation on the node completed.
	ToolkitReady bool `json:"toolkitReady,omitempty"`
	// LastRun stores time of the last bootstrap reconciliation.
	LastRun *metav1.Time `json:"lastRun,omitempty"`
	// Workloads lists health state of every bootstrap workload on the node.
	Workloads []GPUNodeBootstrapWorkloadStatus `json:"workloads,omitempty"`
	// ValidatorRequired is true when at least one GPU still needs validation.
	ValidatorRequired bool `json:"validatorRequired,omitempty"`
	// PendingDevices lists inventory IDs of devices awaiting validation.
	PendingDevices []string `json:"pendingDevices,omitempty"`
	// Validations tracks validation attempts per device.
	Validations []GPUNodeValidationState `json:"validations,omitempty"`
}

// GPUNodeBootstrapWorkloadStatus describes individual bootstrap workload health.
type GPUNodeBootstrapWorkloadStatus struct {
	// Name matches the bootstrap component identifier (validator, gpu-feature-discovery, etc.).
	Name string `json:"name"`
	// Healthy reports whether the workload is running and ready.
	Healthy bool `json:"healthy"`
	// Message contains human readable diagnostics when Healthy=false.
	Message string `json:"message,omitempty"`
	// Since marks when the workload entered its current state.
	Since *metav1.Time `json:"since,omitempty"`
}

// GPUNodeValidationState stores validation attempts for a device.
type GPUNodeValidationState struct {
	// InventoryID references the GPUDevice inventory identifier.
	InventoryID string `json:"inventoryID"`
	// Attempts counts failed validation attempts.
	Attempts int32 `json:"attempts,omitempty"`
	// LastFailure records the timestamp of the last failed attempt.
	LastFailure *metav1.Time `json:"lastFailure,omitempty"`
}

// +kubebuilder:object:root=true
// GPUNodeInventoryList holds a list of GPUNodeInventory objects.
type GPUNodeInventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUNodeInventory `json:"items"`
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
	// MIGLayout allows specifying per-device MIG profiles with counts (only when Unit=MIG).
	MIGLayout []GPUPoolMIGDeviceLayout `json:"migLayout,omitempty"`
	// MaxDevicesPerNode caps number of devices contributed per node.
	MaxDevicesPerNode *int32 `json:"maxDevicesPerNode,omitempty"`
	// SlicesPerUnit configures oversubscription per base unit (card or MIG partition).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	// +kubebuilder:default:=1
	SlicesPerUnit int32 `json:"slicesPerUnit,omitempty"`
	// TimeSlicingResources allows overriding slicesPerUnit per resource.
	TimeSlicingResources []GPUPoolTimeSlicingResource `json:"timeSlicingResources,omitempty"`
}

// GPUPoolMIGDeviceLayout describes MIG profiles per device selector.
type GPUPoolMIGDeviceLayout struct {
	// UUIDs restricts layout to specific GPU UUIDs.
	UUIDs []string `json:"uuids,omitempty"`
	// PCIBusIDs restricts layout to specific PCI bus IDs (domain:bus:slot.func).
	PCIBusIDs []string `json:"pciBusIDs,omitempty"`
	// DeviceFilter allows matching by vendor:device hex strings (e.g. 0x20B010DE).
	DeviceFilter []string `json:"deviceFilter,omitempty"`
	// Profiles lists MIG profiles with optional counts.
	Profiles []GPUPoolMIGProfile `json:"profiles,omitempty"`
	// SlicesPerUnit overrides resource.slicesPerUnit for this layout group.
	SlicesPerUnit *int32 `json:"slicesPerUnit,omitempty"`
}

// GPUPoolMIGProfile represents desired MIG profile count.
type GPUPoolMIGProfile struct {
	// Name is the MIG profile name (e.g. 1g.10gb).
	Name string `json:"name,omitempty"`
	// Count is how many instances of the profile to create (defaults to 1).
	Count *int32 `json:"count,omitempty"`
	// SlicesPerUnit overrides resource or layout slices for this profile (optional).
	SlicesPerUnit *int32 `json:"slicesPerUnit,omitempty"`
}

// GPUPoolTimeSlicingResource overrides slicesPerUnit for a specific resource.
type GPUPoolTimeSlicingResource struct {
	// Name is the resource name to override (defaults to pool resource name).
	Name string `json:"name,omitempty"`
	// SlicesPerUnit is the number of slices for this resource.
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
	// Capacity summarises total/used/available capacity inside the pool.
	Capacity GPUPoolCapacityStatus `json:"capacity,omitempty"`
	// Nodes lists per-node usage statistics.
	Nodes []GPUPoolNodeStatus `json:"nodes,omitempty"`
	// Candidates enumerates nodes/devices pending for acceptance.
	Candidates []GPUPoolCandidate `json:"candidates,omitempty"`
	// Devices mirrors per-device state for quick pool-centric lookups.
	Devices []GPUPoolDeviceStatus `json:"devices,omitempty"`
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
	// Used is currently consumed capacity.
	Used int32 `json:"used,omitempty"`
	// Available is free capacity that may still be assigned.
	Available int32 `json:"available,omitempty"`
	// Unit repeats the resource unit for clarity.
	Unit string `json:"unit,omitempty"`
	// BaseUnits is the number of base devices/partitions contributing to capacity.
	BaseUnits int32 `json:"baseUnits,omitempty"`
	// SlicesPerUnit shows how many slices each base unit provides.
	SlicesPerUnit int32 `json:"slicesPerUnit,omitempty"`
}

type GPUPoolNodeStatus struct {
	// Name is the node identifier.
	Name string `json:"name,omitempty"`
	// TotalDevices is the number of devices contributing to the pool on this node.
	TotalDevices int32 `json:"totalDevices,omitempty"`
	// AssignedDevices counts devices from this node currently in use.
	AssignedDevices int32 `json:"assignedDevices,omitempty"`
	// Health summarises node-level readiness (e.g. Healthy/Misconfigured).
	Health string `json:"health,omitempty"`
	// LastEvent points to the latest significant event impacting the node in context of the pool.
	LastEvent *metav1.Time `json:"lastEvent,omitempty"`
}

type GPUPoolCandidate struct {
	// Name is the node or device awaiting assignment.
	Name string `json:"name,omitempty"`
	// Pools lists pending pool assignments for this candidate.
	Pools []GPUPoolAssignment `json:"pools,omitempty"`
	// LastEvent records when candidate state last changed.
	LastEvent *metav1.Time `json:"lastEvent,omitempty"`
}

type GPUPoolAssignment struct {
	// Pool is the GPUPool name awaiting assignment.
	Pool string `json:"pool,omitempty"`
	// Reason describes why the assignment is pending or rejected.
	Reason string `json:"reason,omitempty"`
	// AutoApproved indicates whether assignment will be granted automatically.
	AutoApproved bool `json:"autoApproved,omitempty"`
	// AnnotationHint suggests annotation to apply for manual approval.
	AnnotationHint string `json:"annotationHint,omitempty"`
}

type GPUPoolDeviceStatus struct {
	// InventoryID references the GPUDevice included in the pool.
	InventoryID string `json:"inventoryID,omitempty"`
	// Node is the node hosting the device.
	Node string `json:"node,omitempty"`
	// State is the current state of the device inside the pool.
	State GPUDeviceState `json:"state,omitempty"`
	// AutoAttach mirrors the auto-attach flag for the device.
	AutoAttach bool `json:"autoAttach,omitempty"`
}

// +kubebuilder:object:root=true
// GPUPoolList holds a list of GPUPool objects.
type GPUPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GPUPool `json:"items"`
}
