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

package v1alpha1_test

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestDeepCopyCoversAllTypes(t *testing.T) {
	now := metav1.Now()
	migCapable := true

	device := &v1alpha1.GPUDevice{
		TypeMeta: metav1.TypeMeta{Kind: "GPUDevice", APIVersion: v1alpha1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a-0000",
			Labels: map[string]string{"gpu.deckhouse.io/device.index": "0"},
		},
		Spec: v1alpha1.GPUDeviceSpec{},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName:    "node-a",
			InventoryID: "node-a-0000",
			Managed:     true,
			State:       v1alpha1.GPUDeviceStateAssigned,
			AutoAttach:  true,
			PoolRef:     &v1alpha1.GPUPoolReference{Name: "pool-a", Resource: "resources.gpu.deckhouse.io/a"},
			Hardware: v1alpha1.GPUDeviceHardware{
				Product:   "NVIDIA A100",
				PCI:       v1alpha1.PCIAddress{Vendor: "10de", Device: "20b0", Class: "0302"},
				MemoryMiB: 40960,
				ComputeCapability: &v1alpha1.GPUComputeCapability{
					Major: 8,
					Minor: 0,
				},
				Precision: v1alpha1.GPUPrecision{
					Supported: []string{"fp32", "fp16", "bf16"},
				},
				MIG: v1alpha1.GPUMIGConfig{
					Capable:           true,
					Strategy:          v1alpha1.GPUMIGStrategyMixed,
					ProfilesSupported: []string{"1g.10gb", "2g.20gb"},
					Types: []v1alpha1.GPUMIGTypeCapacity{
						{
							Name:            "1g.10gb",
							Count:           2,
							MemoryMiB:       10240,
							Multiprocessors: 7,
							Partition: v1alpha1.GPUMIGPartition{
								GPUInstance:     1,
								ComputeInstance: 2,
							},
							Engines: v1alpha1.GPUMIGEngines{
								Copy:    2,
								Encoder: 1,
								Decoder: 1,
								OFAs:    1,
							},
						},
					},
				},
			},
			Health: v1alpha1.GPUDeviceHealth{
				TemperatureC:    42,
				ECCErrorsTotal:  1,
				LastUpdatedTime: &now,
				Metrics: map[string]string{
					"powerW": "300",
				},
			},
			Conditions: []metav1.Condition{{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
			}},
		},
	}

	deviceCopy := device.DeepCopy()
	if deviceCopy == device {
		t.Fatal("DeepCopy must return a different pointer")
	}
	if !reflect.DeepEqual(deviceCopy, device) {
		t.Fatalf("DeepCopy should preserve content:\nwant %#v\ngot  %#v", device, deviceCopy)
	}
	deviceCopy.Status.Hardware.Product = "mutated"
	if device.Status.Hardware.Product == "mutated" {
		t.Fatal("DeepCopy should create independent hardware struct")
	}

	var deviceInto v1alpha1.GPUDevice
	device.DeepCopyInto(&deviceInto)
	if !reflect.DeepEqual(deviceInto.Status.Hardware.PCI, device.Status.Hardware.PCI) {
		t.Fatal("DeepCopyInto must copy nested structs")
	}

	if obj := device.DeepCopyObject(); obj == nil {
		t.Fatal("DeepCopyObject must not return nil")
	}

	devices := &v1alpha1.GPUDeviceList{
		TypeMeta: metav1.TypeMeta{Kind: "GPUDeviceList", APIVersion: v1alpha1.GroupVersion.String()},
		Items:    []v1alpha1.GPUDevice{*device},
	}
	if copy := devices.DeepCopy(); len(copy.Items) != 1 || copy.Items[0].Status.InventoryID != device.Status.InventoryID {
		t.Fatal("GPUDeviceList DeepCopy must copy items")
	}
	if obj := devices.DeepCopyObject(); obj == nil {
		t.Fatal("GPUDeviceList DeepCopyObject must not be nil")
	}

	nodeInventory := &v1alpha1.GPUNodeInventory{
		TypeMeta: metav1.TypeMeta{Kind: "GPUNodeInventory", APIVersion: v1alpha1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Spec: v1alpha1.GPUNodeInventorySpec{NodeName: "node-a"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Present: true,
				Devices: []v1alpha1.GPUNodeDevice{
					{
						InventoryID: device.Status.InventoryID,
						Product:     device.Status.Hardware.Product,
						PCI:         device.Status.Hardware.PCI,
						MemoryMiB:   40960,
						MIG:         device.Status.Hardware.MIG,
						UUID:        "GPU-uuid",
						ComputeCap:  device.Status.Hardware.ComputeCapability,
						Precision:   device.Status.Hardware.Precision,
						State:       v1alpha1.GPUDeviceStateAssigned,
						AutoAttach:  true,
						Reason:      "in-use",
						Health:      device.Status.Health,
					},
				},
			},
			Driver: v1alpha1.GPUNodeDriver{
				Version:      "535.161.08",
				CUDAVersion:  "12.4",
				ToolkitReady: true,
			},
			Monitoring: v1alpha1.GPUNodeMonitoring{
				DCGMReady:     true,
				LastHeartbeat: &now,
			},
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				GFDReady:     true,
				ToolkitReady: true,
				LastRun:      &now,
			},
			Pools: v1alpha1.GPUNodePoolsStatus{
				Assigned: []v1alpha1.GPUNodeAssignedPool{
					{
						Name:          "pool-a",
						Resource:      "resources.gpu.deckhouse.io/a",
						SlotsReserved: 2,
						Since:         &now,
					},
				},
				Pending: []v1alpha1.GPUNodePendingPool{
					{
						Pool:           "pool-b",
						AutoApproved:   false,
						Reason:         "awaiting-approval",
						AnnotationHint: "gpu.deckhouse.io/approve=pool-b",
					},
				},
			},
			Conditions: []metav1.Condition{{
				Type:               "ReadyForPooling",
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 2,
			}},
		},
	}

	if copy := nodeInventory.DeepCopy(); !reflect.DeepEqual(copy.Status.Pools, nodeInventory.Status.Pools) {
		t.Fatal("GPUNodeInventory DeepCopy must copy pools status")
	}
	if obj := nodeInventory.DeepCopyObject(); obj == nil {
		t.Fatal("GPUNodeInventory DeepCopyObject must not be nil")
	}

	nodeList := &v1alpha1.GPUNodeInventoryList{
		TypeMeta: metav1.TypeMeta{Kind: "GPUNodeInventoryList", APIVersion: v1alpha1.GroupVersion.String()},
		Items:    []v1alpha1.GPUNodeInventory{*nodeInventory},
	}
	if copy := nodeList.DeepCopy(); len(copy.Items) != 1 || copy.Items[0].Spec.NodeName != "node-a" {
		t.Fatal("GPUNodeInventoryList DeepCopy must copy entries")
	}
	if obj := nodeList.DeepCopyObject(); obj == nil {
		t.Fatal("GPUNodeInventoryList DeepCopyObject must not be nil")
	}

	pool := &v1alpha1.GPUPool{
		TypeMeta: metav1.TypeMeta{Kind: "GPUPool", APIVersion: v1alpha1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool-a",
		},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "Nvidia",
			Backend:  "DevicePlugin",
			Resource: v1alpha1.GPUPoolResourceSpec{
				Unit:       "MIG",
				MIGProfile: "1g.10gb",
				MIGLayout: []v1alpha1.GPUPoolMIGDeviceLayout{{
					UUIDs: []string{"GPU-uuid"},
					Profiles: []v1alpha1.GPUPoolMIGProfile{{
						Name:  "1g.10gb",
						Count: func() *int32 { v := int32(2); return &v }(),
					}},
					SlicesPerUnit: func() *int32 { v := int32(2); return &v }(),
				}},
				MaxDevicesPerNode: func() *int32 { v := int32(2); return &v }(),
				SlicesPerUnit:     4,
				TimeSlicingResources: []v1alpha1.GPUPoolTimeSlicingResource{{
					Name:          "resources.gpu.deckhouse.io/a",
					SlicesPerUnit: 3,
				}},
			},
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"gpu.deckhouse.io/role": "compute"},
			},
			DeviceSelector: &v1alpha1.GPUPoolDeviceSelector{
				Include: v1alpha1.GPUPoolSelectorRules{
					InventoryIDs: []string{"node-a-0000"},
					Products:     []string{"NVIDIA A100"},
					PCIVendors:   []string{"10de"},
					PCIDevices:   []string{"20b0"},
					MIGCapable:   &migCapable,
					MIGProfiles:  []string{"1g.10gb"},
				},
				Exclude: v1alpha1.GPUPoolSelectorRules{
					Products: []string{"NVIDIA T4"},
				},
			},
			DeviceAssignment: v1alpha1.GPUPoolAssignmentSpec{
				RequireAnnotation: true,
				AutoApproveSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "node-role.kubernetes.io/gpu",
						Operator: metav1.LabelSelectorOpExists,
					}},
				},
			},
			Access: v1alpha1.GPUPoolAccessSpec{
				Namespaces:      []string{"ml"},
				ServiceAccounts: []string{"ml:trainer"},
				DexGroups:       []string{"ml-team"},
			},
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
				Taints: []v1alpha1.GPUPoolTaintSpec{{
					Key:    "gpu.deckhouse.io/pool",
					Value:  "pool-a",
					Effect: "NoSchedule",
				}},
			},
		},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{
				Total:     4,
				Used:      1,
				Available: 3,
				Unit:      "Card",
			},
			Nodes: []v1alpha1.GPUPoolNodeStatus{{
				Name:            "node-a",
				TotalDevices:    2,
				AssignedDevices: 1,
				Health:          "Healthy",
				LastEvent:       &now,
			}},
			Candidates: []v1alpha1.GPUPoolCandidate{{
				Name:      "node-b",
				Pools:     []v1alpha1.GPUPoolAssignment{{Pool: "pool-a", Reason: "pending"}},
				LastEvent: &now,
			}},
			Devices: []v1alpha1.GPUPoolDeviceStatus{{
				InventoryID: device.Status.InventoryID,
				State:       v1alpha1.GPUDeviceStateAssigned,
			}},
			Conditions: []metav1.Condition{{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			}},
		},
	}

	if copy := pool.DeepCopy(); !reflect.DeepEqual(copy.Spec.Resource, pool.Spec.Resource) {
		t.Fatal("GPUPool DeepCopy must copy resource spec")
	}
	if obj := pool.DeepCopyObject(); obj == nil {
		t.Fatal("GPUPool DeepCopyObject must not return nil")
	}

	poolList := &v1alpha1.GPUPoolList{
		TypeMeta: metav1.TypeMeta{Kind: "GPUPoolList", APIVersion: v1alpha1.GroupVersion.String()},
		Items:    []v1alpha1.GPUPool{*pool},
	}
	if copy := poolList.DeepCopy(); len(copy.Items) != 1 || copy.Items[0].Spec.Resource.Unit != pool.Spec.Resource.Unit {
		t.Fatal("GPUPoolList DeepCopy must copy items")
	}
	if obj := poolList.DeepCopyObject(); obj == nil {
		t.Fatal("GPUPoolList DeepCopyObject must not return nil")
	}

	// Directly exercise standalone DeepCopy helpers that are not triggered by the composite objects above.
	if spec := (&v1alpha1.GPUDeviceSpec{}).DeepCopy(); spec == nil {
		t.Fatal("GPUDeviceSpec DeepCopy should return non-nil")
	}
	if cc := (&v1alpha1.GPUComputeCapability{Major: 9, Minor: 0}).DeepCopy(); cc == nil || cc.Major != 9 {
		t.Fatal("GPUComputeCapability DeepCopy must copy fields")
	}
	if mig := (&v1alpha1.GPUMIGConfig{}).DeepCopy(); mig == nil {
		t.Fatal("GPUMIGConfig DeepCopy should return non-nil")
	}
	if precision := (&v1alpha1.GPUPrecision{Supported: []string{"fp32"}}).DeepCopy(); len(precision.Supported) != 1 {
		t.Fatal("GPUPrecision DeepCopy must copy slice")
	}
	if partition := (&v1alpha1.GPUMIGPartition{GPUInstance: 1}).DeepCopy(); partition.GPUInstance != 1 {
		t.Fatal("GPUMIGPartition DeepCopy must copy values")
	}
	if engines := (&v1alpha1.GPUMIGEngines{Copy: 1}).DeepCopy(); engines.Copy != 1 {
		t.Fatal("GPUMIGEngines DeepCopy must copy values")
	}
	if ref := (&v1alpha1.GPUPoolReference{Name: "pool"}).DeepCopy(); ref.Name != "pool" {
		t.Fatal("GPUPoolReference DeepCopy must copy fields")
	}
	if rules := (&v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"a"}, MIGCapable: &migCapable, MIGProfiles: []string{"1g.10gb"}}).DeepCopy(); len(rules.InventoryIDs) != 1 || rules.MIGCapable == nil || !*rules.MIGCapable || len(rules.MIGProfiles) != 1 {
		t.Fatal("GPUPoolSelectorRules DeepCopy must copy fields")
	}
	if assign := (&v1alpha1.GPUPoolAssignment{Pool: "pool"}).DeepCopy(); assign.Pool != "pool" {
		t.Fatal("GPUPoolAssignment DeepCopy must copy fields")
	}
	if assignSpec := (&v1alpha1.GPUPoolAssignmentSpec{RequireAnnotation: true}).DeepCopy(); !assignSpec.RequireAnnotation {
		t.Fatal("GPUPoolAssignmentSpec DeepCopy must copy flag")
	}
	if pending := (&v1alpha1.GPUNodePendingPool{Pool: "pool"}).DeepCopy(); pending.Pool != "pool" {
		t.Fatal("GPUNodePendingPool DeepCopy must copy fields")
	}
	if assigned := (&v1alpha1.GPUNodeAssignedPool{Name: "pool"}).DeepCopy(); assigned.Name != "pool" {
		t.Fatal("GPUNodeAssignedPool DeepCopy must copy fields")
	}
	if monitoring := (&v1alpha1.GPUNodeMonitoring{DCGMReady: true}).DeepCopy(); !monitoring.DCGMReady {
		t.Fatal("GPUNodeMonitoring DeepCopy must copy fields")
	}
	if bootstrap := (&v1alpha1.GPUNodeBootstrapStatus{GFDReady: true}).DeepCopy(); !bootstrap.GFDReady {
		t.Fatal("GPUNodeBootstrapStatus DeepCopy must copy fields")
	}
	if pools := (&v1alpha1.GPUNodePoolsStatus{Assigned: []v1alpha1.GPUNodeAssignedPool{{Name: "pool"}}}).DeepCopy(); len(pools.Assigned) != 1 {
		t.Fatal("GPUNodePoolsStatus DeepCopy must copy slices")
	}
	if hardware := (&v1alpha1.GPUNodeHardware{Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "id"}}}).DeepCopy(); len(hardware.Devices) != 1 {
		t.Fatal("GPUNodeHardware DeepCopy must copy devices")
	}

	// Exercise nil receivers to cover early returns.
	var (
		nilDevice                *v1alpha1.GPUDevice
		nilDeviceList            *v1alpha1.GPUDeviceList
		nilDeviceHardware        *v1alpha1.GPUDeviceHardware
		nilDeviceHealth          *v1alpha1.GPUDeviceHealth
		nilDeviceStatus          *v1alpha1.GPUDeviceStatus
		nilMIGConfig             *v1alpha1.GPUMIGConfig
		nilNodeInventory         *v1alpha1.GPUNodeInventory
		nilNodeInventoryList     *v1alpha1.GPUNodeInventoryList
		nilNodeInventoryStatus   *v1alpha1.GPUNodeInventoryStatus
		nilPool                  *v1alpha1.GPUPool
		nilPoolList              *v1alpha1.GPUPoolList
		nilPoolStatus            *v1alpha1.GPUPoolStatus
		nilPrecision             *v1alpha1.GPUPrecision
		nilComputeCapability     *v1alpha1.GPUComputeCapability
		nilPoolSelectorRules     *v1alpha1.GPUPoolSelectorRules
		nilPoolAssignment        *v1alpha1.GPUPoolAssignment
		nilPoolAssignmentSpec    *v1alpha1.GPUPoolAssignmentSpec
		nilPoolDeviceSelector    *v1alpha1.GPUPoolDeviceSelector
		nilPoolSchedulingSpec    *v1alpha1.GPUPoolSchedulingSpec
		nilPoolAccessSpec        *v1alpha1.GPUPoolAccessSpec
		nilPoolResourceSpec      *v1alpha1.GPUPoolResourceSpec
		nilPoolCandidate         *v1alpha1.GPUPoolCandidate
		nilPoolCapacityStatus    *v1alpha1.GPUPoolCapacityStatus
		nilPoolDeviceStatus      *v1alpha1.GPUPoolDeviceStatus
		nilPoolNodeStatus        *v1alpha1.GPUPoolNodeStatus
		nilNodePendingPool       *v1alpha1.GPUNodePendingPool
		nilNodeAssignedPool      *v1alpha1.GPUNodeAssignedPool
		nilNodeBootstrapStatus   *v1alpha1.GPUNodeBootstrapStatus
		nilNodeMonitoring        *v1alpha1.GPUNodeMonitoring
		nilNodePoolsStatus       *v1alpha1.GPUNodePoolsStatus
		nilNodeHardware          *v1alpha1.GPUNodeHardware
		nilNodeDevice            *v1alpha1.GPUNodeDevice
		nilNodeDriver            *v1alpha1.GPUNodeDriver
		nilPoolReference         *v1alpha1.GPUPoolReference
		nilPoolSpec              *v1alpha1.GPUPoolSpec
		nilPoolTaintSpec         *v1alpha1.GPUPoolTaintSpec
		nilPoolScheduling        *v1alpha1.GPUPoolSchedulingSpec
		nilPoolDeviceSelectorPtr *v1alpha1.GPUPoolDeviceSelector
		nilPartition             *v1alpha1.GPUMIGPartition
		nilEngines               *v1alpha1.GPUMIGEngines
		nilTypeCapacity          *v1alpha1.GPUMIGTypeCapacity
		nilInventorySpec         *v1alpha1.GPUNodeInventorySpec
		nilDeviceSpec            *v1alpha1.GPUDeviceSpec
		nilPCIAddress            *v1alpha1.PCIAddress
	)

	if nilDevice.DeepCopy() != nil {
		t.Fatal("nil GPUDevice DeepCopy must return nil")
	}
	if nilDevice.DeepCopyObject() != nil {
		t.Fatal("nil GPUDevice DeepCopyObject must return nil")
	}
	if nilDeviceList.DeepCopy() != nil {
		t.Fatal("nil GPUDeviceList DeepCopy must return nil")
	}
	if nilDeviceList.DeepCopyObject() != nil {
		t.Fatal("nil GPUDeviceList DeepCopyObject must return nil")
	}
	if nilDeviceHardware.DeepCopy() != nil {
		t.Fatal("nil GPUDeviceHardware DeepCopy must return nil")
	}
	if nilDeviceHealth.DeepCopy() != nil {
		t.Fatal("nil GPUDeviceHealth DeepCopy must return nil")
	}
	if nilDeviceStatus.DeepCopy() != nil {
		t.Fatal("nil GPUDeviceStatus DeepCopy must return nil")
	}
	if nilMIGConfig.DeepCopy() != nil {
		t.Fatal("nil GPUMIGConfig DeepCopy must return nil")
	}
	if nilNodeInventory.DeepCopy() != nil {
		t.Fatal("nil GPUNodeInventory DeepCopy must return nil")
	}
	if nilNodeInventory.DeepCopyObject() != nil {
		t.Fatal("nil GPUNodeInventory DeepCopyObject must return nil")
	}
	if nilNodeInventoryList.DeepCopy() != nil {
		t.Fatal("nil GPUNodeInventoryList DeepCopy must return nil")
	}
	if nilNodeInventoryList.DeepCopyObject() != nil {
		t.Fatal("nil GPUNodeInventoryList DeepCopyObject must return nil")
	}
	if nilNodeInventoryStatus.DeepCopy() != nil {
		t.Fatal("nil GPUNodeInventoryStatus DeepCopy must return nil")
	}
	if nilPool.DeepCopy() != nil {
		t.Fatal("nil GPUPool DeepCopy must return nil")
	}
	if nilPool.DeepCopyObject() != nil {
		t.Fatal("nil GPUPool DeepCopyObject must return nil")
	}
	if nilPoolList.DeepCopy() != nil {
		t.Fatal("nil GPUPoolList DeepCopy must return nil")
	}
	if nilPoolList.DeepCopyObject() != nil {
		t.Fatal("nil GPUPoolList DeepCopyObject must return nil")
	}
	if nilPoolStatus.DeepCopy() != nil {
		t.Fatal("nil GPUPoolStatus DeepCopy must return nil")
	}
	if nilPrecision.DeepCopy() != nil {
		t.Fatal("nil GPUPrecision DeepCopy must return nil")
	}
	if nilComputeCapability.DeepCopy() != nil {
		t.Fatal("nil GPUComputeCapability DeepCopy must return nil")
	}
	if nilPoolSelectorRules.DeepCopy() != nil {
		t.Fatal("nil GPUPoolSelectorRules DeepCopy must return nil")
	}
	if nilPoolAssignment.DeepCopy() != nil {
		t.Fatal("nil GPUPoolAssignment DeepCopy must return nil")
	}
	if nilPoolAssignmentSpec.DeepCopy() != nil {
		t.Fatal("nil GPUPoolAssignmentSpec DeepCopy must return nil")
	}
	if nilPoolDeviceSelector.DeepCopy() != nil {
		t.Fatal("nil GPUPoolDeviceSelector DeepCopy must return nil")
	}
	if nilPoolSchedulingSpec.DeepCopy() != nil {
		t.Fatal("nil GPUPoolSchedulingSpec DeepCopy must return nil")
	}
	if nilPoolAccessSpec.DeepCopy() != nil {
		t.Fatal("nil GPUPoolAccessSpec DeepCopy must return nil")
	}
	if nilPoolResourceSpec.DeepCopy() != nil {
		t.Fatal("nil GPUPoolResourceSpec DeepCopy must return nil")
	}
	if nilPoolCandidate.DeepCopy() != nil {
		t.Fatal("nil GPUPoolCandidate DeepCopy must return nil")
	}
	if nilPoolCapacityStatus.DeepCopy() != nil {
		t.Fatal("nil GPUPoolCapacityStatus DeepCopy must return nil")
	}
	if nilPoolDeviceStatus.DeepCopy() != nil {
		t.Fatal("nil GPUPoolDeviceStatus DeepCopy must return nil")
	}
	if nilPoolNodeStatus.DeepCopy() != nil {
		t.Fatal("nil GPUPoolNodeStatus DeepCopy must return nil")
	}
	if nilNodePendingPool.DeepCopy() != nil {
		t.Fatal("nil GPUNodePendingPool DeepCopy must return nil")
	}
	if nilNodeAssignedPool.DeepCopy() != nil {
		t.Fatal("nil GPUNodeAssignedPool DeepCopy must return nil")
	}
	if nilNodeBootstrapStatus.DeepCopy() != nil {
		t.Fatal("nil GPUNodeBootstrapStatus DeepCopy must return nil")
	}
	if nilNodeMonitoring.DeepCopy() != nil {
		t.Fatal("nil GPUNodeMonitoring DeepCopy must return nil")
	}
	if nilNodePoolsStatus.DeepCopy() != nil {
		t.Fatal("nil GPUNodePoolsStatus DeepCopy must return nil")
	}
	if nilNodeHardware.DeepCopy() != nil {
		t.Fatal("nil GPUNodeHardware DeepCopy must return nil")
	}
	if nilNodeDevice.DeepCopy() != nil {
		t.Fatal("nil GPUNodeDevice DeepCopy must return nil")
	}
	if nilNodeDriver.DeepCopy() != nil {
		t.Fatal("nil GPUNodeDriver DeepCopy must return nil")
	}
	if nilPoolReference.DeepCopy() != nil {
		t.Fatal("nil GPUPoolReference DeepCopy must return nil")
	}
	if nilPoolSpec.DeepCopy() != nil {
		t.Fatal("nil GPUPoolSpec DeepCopy must return nil")
	}
	if nilPoolTaintSpec.DeepCopy() != nil {
		t.Fatal("nil GPUPoolTaintSpec DeepCopy must return nil")
	}
	if nilPoolScheduling.DeepCopy() != nil {
		t.Fatal("nil GPUPoolSchedulingSpec DeepCopy must return nil")
	}
	if nilPoolDeviceSelectorPtr.DeepCopy() != nil {
		t.Fatal("nil GPUPoolDeviceSelector pointer DeepCopy must return nil")
	}
	if nilPartition.DeepCopy() != nil {
		t.Fatal("nil GPUMIGPartition DeepCopy must return nil")
	}
	if nilEngines.DeepCopy() != nil {
		t.Fatal("nil GPUMIGEngines DeepCopy must return nil")
	}
	if nilTypeCapacity.DeepCopy() != nil {
		t.Fatal("nil GPUMIGTypeCapacity DeepCopy must return nil")
	}
	if nilInventorySpec.DeepCopy() != nil {
		t.Fatal("nil GPUNodeInventorySpec DeepCopy must return nil")
	}
	if nilDeviceSpec.DeepCopy() != nil {
		t.Fatal("nil GPUDeviceSpec DeepCopy must return nil")
	}
	if nilPCIAddress.DeepCopy() != nil {
		t.Fatal("nil PCIAddress DeepCopy must return nil")
	}

	// Explicit DeepCopy calls on populated instances to cover non-nil paths.
	if dc := (&device.Status).DeepCopy(); dc == nil || dc.NodeName != "node-a" {
		t.Fatal("GPUDeviceStatus DeepCopy must copy fields")
	}
	if hw := (&device.Status.Hardware).DeepCopy(); hw == nil || hw.Product != "NVIDIA A100" {
		t.Fatal("GPUDeviceHardware DeepCopy must copy fields")
	}
	if pci := (&device.Status.Hardware.PCI).DeepCopy(); pci == nil || pci.Vendor == "" {
		t.Fatal("PCIAddress DeepCopy must copy vendor")
	}
	if cc := device.Status.Hardware.ComputeCapability.DeepCopy(); cc == nil || cc.Major != 8 {
		t.Fatal("GPUComputeCapability DeepCopy must copy values")
	}
	if prec := (&device.Status.Hardware.Precision).DeepCopy(); prec == nil || len(prec.Supported) == 0 {
		t.Fatal("GPUPrecision DeepCopy must copy supported list")
	}
	if mig := (&device.Status.Hardware.MIG).DeepCopy(); mig == nil || len(mig.Types) == 0 {
		t.Fatal("GPUMIGConfig DeepCopy must copy types")
	}
	if migType := (&device.Status.Hardware.MIG.Types[0]).DeepCopy(); migType == nil || migType.Name == "" {
		t.Fatal("GPUMIGTypeCapacity DeepCopy must copy contents")
	}
	if health := (&device.Status.Health).DeepCopy(); health == nil || health.TemperatureC == 0 {
		t.Fatal("GPUDeviceHealth DeepCopy must copy fields")
	}
	if ref := device.Status.PoolRef.DeepCopy(); ref == nil || ref.Name == "" {
		t.Fatal("GPUPoolReference DeepCopy must copy fields")
	}

	if invStatus := (&nodeInventory.Status).DeepCopy(); invStatus == nil || !invStatus.Hardware.Present {
		t.Fatal("GPUNodeInventoryStatus DeepCopy must copy hardware")
	}
	if invSpec := (&nodeInventory.Spec).DeepCopy(); invSpec == nil || invSpec.NodeName == "" {
		t.Fatal("GPUNodeInventorySpec DeepCopy must copy node name")
	}
	if invHw := (&nodeInventory.Status.Hardware).DeepCopy(); invHw == nil || len(invHw.Devices) == 0 {
		t.Fatal("GPUNodeHardware DeepCopy must copy devices")
	}
	if invDev := (&nodeInventory.Status.Hardware.Devices[0]).DeepCopy(); invDev == nil || invDev.InventoryID == "" {
		t.Fatal("GPUNodeDevice DeepCopy must copy fields")
	}
	if invDriver := (&nodeInventory.Status.Driver).DeepCopy(); invDriver == nil || invDriver.Version == "" {
		t.Fatal("GPUNodeDriver DeepCopy must copy version")
	}
	if invMonitoring := (&nodeInventory.Status.Monitoring).DeepCopy(); invMonitoring == nil || !invMonitoring.DCGMReady {
		t.Fatal("GPUNodeMonitoring DeepCopy must copy readiness")
	}
	if invBootstrap := (&nodeInventory.Status.Bootstrap).DeepCopy(); invBootstrap == nil || !invBootstrap.GFDReady {
		t.Fatal("GPUNodeBootstrapStatus DeepCopy must copy readiness")
	}
	if invPools := (&nodeInventory.Status.Pools).DeepCopy(); invPools == nil || len(invPools.Assigned) == 0 {
		t.Fatal("GPUNodePoolsStatus DeepCopy must copy pools")
	}
	if invAssigned := (&nodeInventory.Status.Pools.Assigned[0]).DeepCopy(); invAssigned == nil || invAssigned.Name == "" {
		t.Fatal("GPUNodeAssignedPool DeepCopy must copy name")
	}
	if invPending := (&nodeInventory.Status.Pools.Pending[0]).DeepCopy(); invPending == nil || invPending.Pool == "" {
		t.Fatal("GPUNodePendingPool DeepCopy must copy pool")
	}

	if poolSpec := (&pool.Spec).DeepCopy(); poolSpec == nil || poolSpec.Resource.Unit == "" {
		t.Fatal("GPUPoolSpec DeepCopy must copy resource")
	}
	if poolResource := (&pool.Spec.Resource).DeepCopy(); poolResource == nil || poolResource.Unit == "" {
		t.Fatal("GPUPoolResourceSpec DeepCopy must copy resource unit")
	}
	if len(pool.Spec.Resource.MIGLayout) == 0 || len(pool.Spec.Resource.TimeSlicingResources) == 0 {
		t.Fatal("GPUPoolResourceSpec should contain MIGLayout and TimeSlicingResources in test")
	}
	if poolResource := (&pool.Spec.Resource).DeepCopy(); len(poolResource.MIGLayout) == 0 || len(poolResource.TimeSlicingResources) == 0 {
		t.Fatal("GPUPoolResourceSpec DeepCopy must copy MIGLayout and TimeSlicingResources")
	}
	if poolSelector := pool.Spec.DeviceSelector.DeepCopy(); poolSelector == nil || len(poolSelector.Include.InventoryIDs) == 0 || poolSelector.Include.MIGCapable == nil || !*poolSelector.Include.MIGCapable {
		t.Fatal("GPUPoolDeviceSelector DeepCopy must copy include rules")
	}
	if poolRules := (&pool.Spec.DeviceSelector.Include).DeepCopy(); poolRules == nil || len(poolRules.InventoryIDs) == 0 || poolRules.MIGCapable == nil || !*poolRules.MIGCapable {
		t.Fatal("GPUPoolSelectorRules DeepCopy must copy fields")
	}
	if poolAssignment := (&pool.Spec.DeviceAssignment).DeepCopy(); poolAssignment == nil || !poolAssignment.RequireAnnotation {
		t.Fatal("GPUPoolAssignmentSpec DeepCopy must copy flag")
	}
	if poolAccess := (&pool.Spec.Access).DeepCopy(); poolAccess == nil || len(poolAccess.Namespaces) == 0 {
		t.Fatal("GPUPoolAccessSpec DeepCopy must copy namespaces")
	}
	if poolScheduling := (&pool.Spec.Scheduling).DeepCopy(); poolScheduling == nil || len(poolScheduling.Taints) == 0 {
		t.Fatal("GPUPoolSchedulingSpec DeepCopy must copy taints")
	}
	if poolTaint := (&pool.Spec.Scheduling.Taints[0]).DeepCopy(); poolTaint == nil || poolTaint.Key == "" {
		t.Fatal("GPUPoolTaintSpec DeepCopy must copy key")
	}
	if poolStatusDeep := (&pool.Status).DeepCopy(); poolStatusDeep == nil || poolStatusDeep.Capacity.Total == 0 {
		t.Fatal("GPUPoolStatus DeepCopy must copy capacity")
	}
	if poolCapacity := (&pool.Status.Capacity).DeepCopy(); poolCapacity == nil || poolCapacity.Total == 0 {
		t.Fatal("GPUPoolCapacityStatus DeepCopy must copy totals")
	}
	if poolDeviceStatus := (&pool.Status.Devices[0]).DeepCopy(); poolDeviceStatus == nil || poolDeviceStatus.InventoryID == "" {
		t.Fatal("GPUPoolDeviceStatus DeepCopy must copy ID")
	}
	if poolNodeStatus := (&pool.Status.Nodes[0]).DeepCopy(); poolNodeStatus == nil || poolNodeStatus.Name == "" {
		t.Fatal("GPUPoolNodeStatus DeepCopy must copy name")
	}
	if poolCandidate := (&pool.Status.Candidates[0]).DeepCopy(); poolCandidate == nil || poolCandidate.Name == "" {
		t.Fatal("GPUPoolCandidate DeepCopy must copy name")
	}

	// Explicitly invoke DeepCopyInto for types with simple assignments to ensure coverage counters are incremented.
	srcTypeCapacity := &v1alpha1.GPUMIGTypeCapacity{
		Name:            "1g.10gb",
		Count:           1,
		MemoryMiB:       10240,
		Multiprocessors: 7,
		Partition:       v1alpha1.GPUMIGPartition{GPUInstance: 1, ComputeInstance: 2},
		Engines:         v1alpha1.GPUMIGEngines{Copy: 1, Encoder: 1, Decoder: 1, OFAs: 1},
	}
	srcTypeCapacity.DeepCopyInto(&v1alpha1.GPUMIGTypeCapacity{})

	srcAssigned := &v1alpha1.GPUNodeAssignedPool{Name: "pool", Since: &now}
	srcAssigned.DeepCopyInto(&v1alpha1.GPUNodeAssignedPool{})

	srcPending := &v1alpha1.GPUNodePendingPool{Pool: "pool", Reason: "pending"}
	srcPending.DeepCopyInto(&v1alpha1.GPUNodePendingPool{})

	srcBootstrap := &v1alpha1.GPUNodeBootstrapStatus{GFDReady: true, LastRun: &now}
	srcBootstrap.DeepCopyInto(&v1alpha1.GPUNodeBootstrapStatus{})

	srcMonitoring := &v1alpha1.GPUNodeMonitoring{DCGMReady: true, LastHeartbeat: &now}
	srcMonitoring.DeepCopyInto(&v1alpha1.GPUNodeMonitoring{})

	srcPools := &v1alpha1.GPUNodePoolsStatus{
		Assigned: []v1alpha1.GPUNodeAssignedPool{{Name: "pool"}},
		Pending:  []v1alpha1.GPUNodePendingPool{{Pool: "pool-b"}},
	}
	srcPools.DeepCopyInto(&v1alpha1.GPUNodePoolsStatus{})

	srcHardware := &v1alpha1.GPUNodeHardware{
		Present: true,
		Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "node-a-0000"}},
	}
	srcHardware.DeepCopyInto(&v1alpha1.GPUNodeHardware{})

	srcPoolStatus := &v1alpha1.GPUPoolStatus{
		Capacity:   v1alpha1.GPUPoolCapacityStatus{Total: 1},
		Nodes:      []v1alpha1.GPUPoolNodeStatus{{Name: "node-a"}},
		Candidates: []v1alpha1.GPUPoolCandidate{{Name: "node-b"}},
		Devices:    []v1alpha1.GPUPoolDeviceStatus{{InventoryID: "node-a-0000"}},
	}
	srcPoolStatus.DeepCopyInto(&v1alpha1.GPUPoolStatus{})

	srcDeviceStatus := &v1alpha1.GPUPoolDeviceStatus{InventoryID: "node-a-0001"}
	srcDeviceStatus.DeepCopyInto(&v1alpha1.GPUPoolDeviceStatus{})

	srcNodeStatus := &v1alpha1.GPUPoolNodeStatus{Name: "node-a", LastEvent: &now}
	srcNodeStatus.DeepCopyInto(&v1alpha1.GPUPoolNodeStatus{})

	srcCandidate := &v1alpha1.GPUPoolCandidate{
		Name:      "node-c",
		Pools:     []v1alpha1.GPUPoolAssignment{{Pool: "pool"}},
		LastEvent: &now,
	}
	srcCandidate.DeepCopyInto(&v1alpha1.GPUPoolCandidate{})

	srcAssignment := &v1alpha1.GPUPoolAssignment{Pool: "pool", Reason: "pending"}
	srcAssignment.DeepCopyInto(&v1alpha1.GPUPoolAssignment{})

	srcAssignmentSpec := &v1alpha1.GPUPoolAssignmentSpec{RequireAnnotation: true}
	srcAssignmentSpec.DeepCopyInto(&v1alpha1.GPUPoolAssignmentSpec{})

	srcSelectorRules := &v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"node-a-0000"}, MIGCapable: &migCapable, MIGProfiles: []string{"1g.10gb"}}
	srcSelectorRules.DeepCopyInto(&v1alpha1.GPUPoolSelectorRules{})

	srcDeviceSelector := &v1alpha1.GPUPoolDeviceSelector{
		Include: v1alpha1.GPUPoolSelectorRules{InventoryIDs: []string{"node-a-0000"}},
		Exclude: v1alpha1.GPUPoolSelectorRules{Products: []string{"NVIDIA T4"}},
	}
	srcDeviceSelector.DeepCopyInto(&v1alpha1.GPUPoolDeviceSelector{})

	srcScheduling := &v1alpha1.GPUPoolSchedulingSpec{
		Strategy:    v1alpha1.GPUPoolSchedulingBinPack,
		TopologyKey: "topology.kubernetes.io/zone",
		Taints:      []v1alpha1.GPUPoolTaintSpec{{Key: "key"}},
	}
	srcScheduling.DeepCopyInto(&v1alpha1.GPUPoolSchedulingSpec{})

	srcAccess := &v1alpha1.GPUPoolAccessSpec{
		Namespaces:      []string{"ml"},
		ServiceAccounts: []string{"ml:trainer"},
		DexGroups:       []string{"ml-team"},
	}
	srcAccess.DeepCopyInto(&v1alpha1.GPUPoolAccessSpec{})

	srcResource := &v1alpha1.GPUPoolResourceSpec{Unit: "device"}
	srcResource.DeepCopyInto(&v1alpha1.GPUPoolResourceSpec{})

	srcTaint := &v1alpha1.GPUPoolTaintSpec{Key: "key", Value: "value", Effect: "NoSchedule"}
	srcTaint.DeepCopyInto(&v1alpha1.GPUPoolTaintSpec{})

	srcNodeDevice := &v1alpha1.GPUNodeDevice{
		InventoryID: "node-a-0000",
		PCI:         v1alpha1.PCIAddress{Vendor: "10de"},
		MIG:         v1alpha1.GPUMIGConfig{Capable: true},
		Precision:   v1alpha1.GPUPrecision{Supported: []string{"fp32"}},
		Health:      v1alpha1.GPUDeviceHealth{TemperatureC: 40},
	}
	srcNodeDevice.DeepCopyInto(&v1alpha1.GPUNodeDevice{})

	srcNodeDriver := &v1alpha1.GPUNodeDriver{Version: "535"}
	srcNodeDriver.DeepCopyInto(&v1alpha1.GPUNodeDriver{})

	srcMonitoring.DeepCopyInto(&v1alpha1.GPUNodeMonitoring{})

	srcInventoryStatus := &v1alpha1.GPUNodeInventoryStatus{
		Hardware:   v1alpha1.GPUNodeHardware{Devices: []v1alpha1.GPUNodeDevice{{InventoryID: "node-a-0000"}}},
		Driver:     v1alpha1.GPUNodeDriver{Version: "535"},
		Monitoring: v1alpha1.GPUNodeMonitoring{DCGMReady: true},
		Bootstrap:  v1alpha1.GPUNodeBootstrapStatus{GFDReady: true},
		Pools:      v1alpha1.GPUNodePoolsStatus{Assigned: []v1alpha1.GPUNodeAssignedPool{{Name: "pool"}}},
		Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
	}
	srcInventoryStatus.DeepCopyInto(&v1alpha1.GPUNodeInventoryStatus{})

	srcInventorySpec := &v1alpha1.GPUNodeInventorySpec{NodeName: "node-a"}
	srcInventorySpec.DeepCopyInto(&v1alpha1.GPUNodeInventorySpec{})

	srcDeviceSpec := &v1alpha1.GPUDeviceSpec{}
	srcDeviceSpec.DeepCopyInto(&v1alpha1.GPUDeviceSpec{})

	srcPCI := &v1alpha1.PCIAddress{Vendor: "10de"}
	srcPCI.DeepCopyInto(&v1alpha1.PCIAddress{})
}
