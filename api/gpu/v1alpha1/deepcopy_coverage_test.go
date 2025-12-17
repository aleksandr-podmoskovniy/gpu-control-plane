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

package v1alpha1

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptrBool(v bool) *bool    { return &v }
func ptrInt32(v int32) *int32 { return &v }

func TestDeepCopyNilReceivers(t *testing.T) {
	var (
		clusterPool     *ClusterGPUPool
		clusterPoolList *ClusterGPUPoolList
		device          *GPUDevice
		deviceList      *GPUDeviceList
		nodeState       *GPUNodeState
		nodeStateList   *GPUNodeStateList
		pool            *GPUPool
		poolList        *GPUPoolList

		deviceHW       *GPUDeviceHardware
		deviceSpec     *GPUDeviceSpec
		deviceStatus   *GPUDeviceStatus
		migConfig      *GPUMIGConfig
		migType        *GPUMIGTypeCapacity
		nodeStateSpec  *GPUNodeStateSpec
		nodeStateSt    *GPUNodeStateStatus
		poolAssign     *GPUPoolAssignmentSpec
		poolCapacity   *GPUPoolCapacityStatus
		poolSelector   *GPUPoolDeviceSelector
		poolRef        *GPUPoolReference
		poolResource   *GPUPoolResourceSpec
		poolScheduling *GPUPoolSchedulingSpec
		poolRules      *GPUPoolSelectorRules
		poolSpec       *GPUPoolSpec
		poolStatus     *GPUPoolStatus
		poolTaint      *GPUPoolTaintSpec
		pciAddr        *PCIAddress
	)

	if clusterPool.DeepCopy() != nil || clusterPool.DeepCopyObject() != nil {
		t.Fatalf("expected ClusterGPUPool nil deepcopy to return nil")
	}
	if clusterPoolList.DeepCopy() != nil || clusterPoolList.DeepCopyObject() != nil {
		t.Fatalf("expected ClusterGPUPoolList nil deepcopy to return nil")
	}
	if device.DeepCopy() != nil || device.DeepCopyObject() != nil {
		t.Fatalf("expected GPUDevice nil deepcopy to return nil")
	}
	if deviceList.DeepCopy() != nil || deviceList.DeepCopyObject() != nil {
		t.Fatalf("expected GPUDeviceList nil deepcopy to return nil")
	}
	if nodeState.DeepCopy() != nil || nodeState.DeepCopyObject() != nil {
		t.Fatalf("expected GPUNodeState nil deepcopy to return nil")
	}
	if nodeStateList.DeepCopy() != nil || nodeStateList.DeepCopyObject() != nil {
		t.Fatalf("expected GPUNodeStateList nil deepcopy to return nil")
	}
	if pool.DeepCopy() != nil || pool.DeepCopyObject() != nil {
		t.Fatalf("expected GPUPool nil deepcopy to return nil")
	}
	if poolList.DeepCopy() != nil || poolList.DeepCopyObject() != nil {
		t.Fatalf("expected GPUPoolList nil deepcopy to return nil")
	}

	if deviceHW.DeepCopy() != nil {
		t.Fatalf("expected GPUDeviceHardware nil deepcopy to return nil")
	}
	if deviceSpec.DeepCopy() != nil {
		t.Fatalf("expected GPUDeviceSpec nil deepcopy to return nil")
	}
	if deviceStatus.DeepCopy() != nil {
		t.Fatalf("expected GPUDeviceStatus nil deepcopy to return nil")
	}
	if migConfig.DeepCopy() != nil {
		t.Fatalf("expected GPUMIGConfig nil deepcopy to return nil")
	}
	if migType.DeepCopy() != nil {
		t.Fatalf("expected GPUMIGTypeCapacity nil deepcopy to return nil")
	}
	if nodeStateSpec.DeepCopy() != nil {
		t.Fatalf("expected GPUNodeStateSpec nil deepcopy to return nil")
	}
	if nodeStateSt.DeepCopy() != nil {
		t.Fatalf("expected GPUNodeStateStatus nil deepcopy to return nil")
	}
	if poolAssign.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolAssignmentSpec nil deepcopy to return nil")
	}
	if poolCapacity.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolCapacityStatus nil deepcopy to return nil")
	}
	if poolSelector.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolDeviceSelector nil deepcopy to return nil")
	}
	if poolRef.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolReference nil deepcopy to return nil")
	}
	if poolResource.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolResourceSpec nil deepcopy to return nil")
	}
	if poolScheduling.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolSchedulingSpec nil deepcopy to return nil")
	}
	if poolRules.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolSelectorRules nil deepcopy to return nil")
	}
	if poolSpec.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolSpec nil deepcopy to return nil")
	}
	if poolStatus.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolStatus nil deepcopy to return nil")
	}
	if poolTaint.DeepCopy() != nil {
		t.Fatalf("expected GPUPoolTaintSpec nil deepcopy to return nil")
	}
	if pciAddr.DeepCopy() != nil {
		t.Fatalf("expected PCIAddress nil deepcopy to return nil")
	}
}

func TestDeepCopyCoversAllGeneratedMethods(t *testing.T) {
	transitionTime := metav1.NewTime(time.Unix(1710000000, 0))

	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"k": "v"},
	}

	maxDevices := ptrInt32(2)
	taintsEnabled := ptrBool(true)
	migCapable := ptrBool(true)

	rules := GPUPoolSelectorRules{
		InventoryIDs: []string{"inv-1"},
		Products:     []string{"GPU Model"},
		PCIVendors:   []string{"10de"},
		PCIDevices:   []string{"1db6"},
		MIGCapable:   migCapable,
		MIGProfiles:  []string{"1g.10gb"},
	}
	if rules.DeepCopy() == nil {
		t.Fatalf("expected GPUPoolSelectorRules.DeepCopy result")
	}

	poolSelector := &GPUPoolDeviceSelector{
		Include: rules,
		Exclude: rules,
	}

	mig := GPUMIGConfig{
		Capable:           true,
		Strategy:          "single",
		ProfilesSupported: []string{"1g.10gb"},
		Types:             []GPUMIGTypeCapacity{{Name: "1g.10gb", Count: 2}},
	}

	hardware := GPUDeviceHardware{
		UUID:    "GPU-UUID",
		Product: "GPU Model",
		PCI:     PCIAddress{Vendor: "10de", Device: "1db6", Class: "0302", Address: "0000:00:01.0"},
		MIG:     mig,
	}

	deviceStatus := GPUDeviceStatus{
		NodeName:    "node-1",
		InventoryID: "node-1/0000:00:01.0",
		Managed:     true,
		State:       GPUDeviceStateAssigned,
		AutoAttach:  true,
		PoolRef:     &GPUPoolReference{Name: "pool", Namespace: "ns"},
		Hardware:    hardware,
		Conditions:  []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, LastTransitionTime: transitionTime}},
	}
	if deviceStatus.DeepCopy() == nil {
		t.Fatalf("expected GPUDeviceStatus.DeepCopy result")
	}
	if deviceStatus.DeepCopy().PoolRef == deviceStatus.PoolRef {
		t.Fatalf("expected GPUDeviceStatus to deep-copy PoolRef")
	}
	if reflect.DeepEqual(deviceStatus.DeepCopy().Hardware.MIG.Types, deviceStatus.Hardware.MIG.Types) && &deviceStatus.DeepCopy().Hardware.MIG.Types[0] == &deviceStatus.Hardware.MIG.Types[0] {
		t.Fatalf("expected GPUDeviceStatus to deep-copy MIG types slice")
	}

	device := &GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev", Labels: map[string]string{"a": "b"}},
		Spec:       GPUDeviceSpec{},
		Status:     deviceStatus,
	}
	if device.DeepCopy() == nil {
		t.Fatalf("expected GPUDevice.DeepCopy result")
	}
	if device.DeepCopyObject() == nil {
		t.Fatalf("expected GPUDevice.DeepCopyObject result")
	}

	deviceList := &GPUDeviceList{Items: []GPUDevice{*device}}
	if deviceList.DeepCopy() == nil {
		t.Fatalf("expected GPUDeviceList.DeepCopy result")
	}
	if deviceList.DeepCopyObject() == nil {
		t.Fatalf("expected GPUDeviceList.DeepCopyObject result")
	}

	poolSpec := GPUPoolSpec{
		Provider: "Nvidia",
		Backend:  "DevicePlugin",
		Resource: GPUPoolResourceSpec{
			Unit:              "Card",
			SlicesPerUnit:     2,
			MaxDevicesPerNode: maxDevices,
		},
		NodeSelector:   selector,
		DeviceSelector: poolSelector,
		DeviceAssignment: GPUPoolAssignmentSpec{
			AutoApproveSelector: selector,
		},
		Scheduling: GPUPoolSchedulingSpec{
			Strategy:      GPUPoolSchedulingSpread,
			TopologyKey:   "topology.kubernetes.io/zone",
			TaintsEnabled: taintsEnabled,
			Taints:        []GPUPoolTaintSpec{{Key: "k", Value: "v", Effect: "NoSchedule"}},
		},
	}
	if poolSpec.DeepCopy() == nil {
		t.Fatalf("expected GPUPoolSpec.DeepCopy result")
	}

	poolStatus := GPUPoolStatus{
		Capacity:   GPUPoolCapacityStatus{Total: 3},
		Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, LastTransitionTime: transitionTime}},
	}
	if poolStatus.DeepCopy() == nil {
		t.Fatalf("expected GPUPoolStatus.DeepCopy result")
	}

	pool := &GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"},
		Spec:       poolSpec,
		Status:     poolStatus,
	}
	if pool.DeepCopy() == nil {
		t.Fatalf("expected GPUPool.DeepCopy result")
	}
	if pool.DeepCopyObject() == nil {
		t.Fatalf("expected GPUPool.DeepCopyObject result")
	}

	poolList := &GPUPoolList{Items: []GPUPool{*pool}}
	if poolList.DeepCopy() == nil {
		t.Fatalf("expected GPUPoolList.DeepCopy result")
	}
	if poolList.DeepCopyObject() == nil {
		t.Fatalf("expected GPUPoolList.DeepCopyObject result")
	}

	clusterPool := &ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       poolSpec,
		Status:     poolStatus,
	}
	if clusterPool.DeepCopy() == nil {
		t.Fatalf("expected ClusterGPUPool.DeepCopy result")
	}
	if clusterPool.DeepCopyObject() == nil {
		t.Fatalf("expected ClusterGPUPool.DeepCopyObject result")
	}

	clusterPoolList := &ClusterGPUPoolList{Items: []ClusterGPUPool{*clusterPool}}
	if clusterPoolList.DeepCopy() == nil {
		t.Fatalf("expected ClusterGPUPoolList.DeepCopy result")
	}
	if clusterPoolList.DeepCopyObject() == nil {
		t.Fatalf("expected ClusterGPUPoolList.DeepCopyObject result")
	}

	nodeStateStatus := GPUNodeStateStatus{
		Conditions: []metav1.Condition{{Type: "ReadyForPooling", Status: metav1.ConditionTrue, LastTransitionTime: transitionTime}},
	}
	if nodeStateStatus.DeepCopy() == nil {
		t.Fatalf("expected GPUNodeStateStatus.DeepCopy result")
	}

	nodeState := &GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec:       GPUNodeStateSpec{NodeName: "node-1"},
		Status:     nodeStateStatus,
	}
	if nodeState.DeepCopy() == nil {
		t.Fatalf("expected GPUNodeState.DeepCopy result")
	}
	if nodeState.DeepCopyObject() == nil {
		t.Fatalf("expected GPUNodeState.DeepCopyObject result")
	}

	nodeStateList := &GPUNodeStateList{Items: []GPUNodeState{*nodeState}}
	if nodeStateList.DeepCopy() == nil {
		t.Fatalf("expected GPUNodeStateList.DeepCopy result")
	}
	if nodeStateList.DeepCopyObject() == nil {
		t.Fatalf("expected GPUNodeStateList.DeepCopyObject result")
	}

	// Cover remaining standalone types.
	if (&GPUDeviceHardware{}).DeepCopy() == nil {
		t.Fatalf("expected GPUDeviceHardware.DeepCopy result")
	}
	if (&GPUDeviceSpec{}).DeepCopy() == nil {
		t.Fatalf("expected GPUDeviceSpec.DeepCopy result")
	}
	if (&GPUDeviceStatus{}).DeepCopy() == nil {
		t.Fatalf("expected GPUDeviceStatus.DeepCopy result")
	}
	if (&GPUMIGConfig{}).DeepCopy() == nil {
		t.Fatalf("expected GPUMIGConfig.DeepCopy result")
	}
	if (&GPUMIGTypeCapacity{}).DeepCopy() == nil {
		t.Fatalf("expected GPUMIGTypeCapacity.DeepCopy result")
	}
	if (&GPUNodeStateSpec{}).DeepCopy() == nil {
		t.Fatalf("expected GPUNodeStateSpec.DeepCopy result")
	}
	if (&GPUNodeStateStatus{}).DeepCopy() == nil {
		t.Fatalf("expected GPUNodeStateStatus.DeepCopy result")
	}
	if (&GPUPoolAssignmentSpec{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolAssignmentSpec.DeepCopy result")
	}
	if (&GPUPoolCapacityStatus{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolCapacityStatus.DeepCopy result")
	}
	if (&GPUPoolDeviceSelector{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolDeviceSelector.DeepCopy result")
	}
	if (&GPUPoolReference{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolReference.DeepCopy result")
	}
	if (&GPUPoolResourceSpec{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolResourceSpec.DeepCopy result")
	}
	if (&GPUPoolSchedulingSpec{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolSchedulingSpec.DeepCopy result")
	}
	if (&GPUPoolSelectorRules{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolSelectorRules.DeepCopy result")
	}
	if (&GPUPoolTaintSpec{}).DeepCopy() == nil {
		t.Fatalf("expected GPUPoolTaintSpec.DeepCopy result")
	}
	if (&PCIAddress{}).DeepCopy() == nil {
		t.Fatalf("expected PCIAddress.DeepCopy result")
	}
}
