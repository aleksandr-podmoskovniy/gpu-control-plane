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

package physicalgpu

import gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"

type dataMetric struct {
	Name   string
	Node   string
	Vendor string
	Model  string
	UUID   string
	PCI    string
}

func newDataMetric(obj *gpuv1alpha1.PhysicalGPU) *dataMetric {
	if obj == nil {
		return nil
	}

	m := &dataMetric{
		Name: obj.Name,
	}

	if obj.Status.NodeInfo != nil {
		m.Node = obj.Status.NodeInfo.NodeName
	}

	if vendor, ok := obj.Labels["gpu.deckhouse.io/vendor"]; ok {
		m.Vendor = vendor
	} else if obj.Status.PCIInfo != nil && obj.Status.PCIInfo.Vendor != nil {
		m.Vendor = obj.Status.PCIInfo.Vendor.Name
	}

	if obj.Status.Capabilities != nil && obj.Status.Capabilities.ProductName != "" {
		m.Model = obj.Status.Capabilities.ProductName
	} else if obj.Status.PCIInfo != nil && obj.Status.PCIInfo.Device != nil {
		m.Model = obj.Status.PCIInfo.Device.Name
	}

	if obj.Status.CurrentState != nil && obj.Status.CurrentState.Nvidia != nil {
		m.UUID = obj.Status.CurrentState.Nvidia.GPUUUID
	}

	if obj.Status.PCIInfo != nil {
		m.PCI = obj.Status.PCIInfo.Address
	}

	return m
}

func (m *dataMetric) labelValues() []string {
	if m == nil {
		return []string{"", "", "", "", "", ""}
	}
	return []string{m.Name, m.Node, m.Vendor, m.Model, m.UUID, m.PCI}
}
