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

package apply

import (
	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

func buildStatus(obj *gpuv1alpha1.PhysicalGPU, dev state.Device, nodeName string, nodeInfo *gpuv1alpha1.NodeInfo) gpuv1alpha1.PhysicalGPUStatus {
	status := obj.Status
	if nodeInfo == nil {
		nodeInfo = &gpuv1alpha1.NodeInfo{}
	}
	nodeInfo.NodeName = nodeName
	status.NodeInfo = nodeInfo

	status.PCIInfo = buildPCIInfo(dev)
	status.CurrentState = buildCurrentState(status.CurrentState, dev.DriverName)

	return status
}

func buildPCIInfo(dev state.Device) *gpuv1alpha1.PCIInfo {
	pci := &gpuv1alpha1.PCIInfo{
		Address: dev.Address,
	}
	if dev.ClassCode != "" || dev.ClassName != "" {
		pci.Class = &gpuv1alpha1.PCIClassInfo{
			Code: dev.ClassCode,
			Name: dev.ClassName,
		}
	}
	if dev.VendorID != "" || dev.VendorName != "" {
		pci.Vendor = &gpuv1alpha1.PCIVendorInfo{
			ID:   dev.VendorID,
			Name: dev.VendorName,
		}
	}
	if dev.DeviceID != "" || dev.DeviceName != "" {
		pci.Device = &gpuv1alpha1.PCIDeviceInfo{
			ID:   dev.DeviceID,
			Name: dev.DeviceName,
		}
	}
	return pci
}

func buildCurrentState(existing *gpuv1alpha1.GPUCurrentState, driverName string) *gpuv1alpha1.GPUCurrentState {
	driverType := driverTypeFromName(driverName)
	if driverType == "" && existing == nil {
		return nil
	}

	current := existing
	if current == nil {
		current = &gpuv1alpha1.GPUCurrentState{}
	}
	if driverType != "" {
		current.DriverType = driverType
	}
	return current
}
