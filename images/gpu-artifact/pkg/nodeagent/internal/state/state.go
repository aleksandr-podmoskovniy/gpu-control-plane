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

package state

import (
	"fmt"
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

const (
	// LabelNode marks PhysicalGPU objects with the node name.
	LabelNode = "gpu.deckhouse.io/node"
	// LabelVendor marks PhysicalGPU objects with vendor name.
	LabelVendor = "gpu.deckhouse.io/vendor"
	// LabelDevice stores a normalized device name (for example "a30-pcie").
	// It is derived from pci.ids when available.
	LabelDevice = "gpu.deckhouse.io/device"
)

// Device represents a GPU-like PCI device detected on the node.
type Device struct {
	Address    string
	ClassCode  string
	ClassName  string
	VendorID   string
	VendorName string
	DeviceID   string
	DeviceName string
}

// State provides access to a node-agent sync snapshot.
type State interface {
	NodeName() string
	NodeInfo() *gpuv1alpha1.NodeInfo
	SetNodeInfo(info *gpuv1alpha1.NodeInfo)
	Devices() []Device
	SetDevices(devices []Device)
	Expected() map[string]Device
}

type state struct {
	nodeName string
	nodeInfo *gpuv1alpha1.NodeInfo
	devices  []Device
	expected map[string]Device
}

// New initializes the state for a single sync loop.
func New(nodeName string) State {
	return &state{
		nodeName: nodeName,
		expected: map[string]Device{},
	}
}

func (s *state) NodeName() string {
	return s.nodeName
}

func (s *state) NodeInfo() *gpuv1alpha1.NodeInfo {
	return s.nodeInfo
}

func (s *state) SetNodeInfo(info *gpuv1alpha1.NodeInfo) {
	s.nodeInfo = info
}

func (s *state) Devices() []Device {
	return s.devices
}

func (s *state) SetDevices(devices []Device) {
	s.devices = devices
	s.expected = make(map[string]Device, len(devices))
	for _, dev := range devices {
		name := PhysicalGPUName(s.nodeName, dev.Address)
		s.expected[name] = dev
	}
}

func (s *state) Expected() map[string]Device {
	return s.expected
}

// LabelsForDevice builds labels for a PhysicalGPU object.
func LabelsForDevice(nodeName string, dev Device) map[string]string {
	labels := map[string]string{
		LabelNode: nodeName,
	}

	if vendor := VendorLabel(dev.VendorID); vendor != "" {
		labels[LabelVendor] = vendor
	}

	if device := DeviceLabel(dev.DeviceName); device != "" {
		labels[LabelDevice] = device
	}

	return labels
}

// PhysicalGPUName returns a stable name for a PhysicalGPU object.
func PhysicalGPUName(nodeName, pciAddress string) string {
	safe := strings.NewReplacer(":", "-", ".", "-", "_", "-").Replace(pciAddress)
	return fmt.Sprintf("%s-%s", nodeName, safe)
}

// VendorName maps PCI vendor ID to the API enum.
// VendorLabel maps PCI vendor ID to the label value.
func VendorLabel(vendorID string) string {
	switch strings.ToLower(vendorID) {
	case "10de":
		return "nvidia"
	case "1002":
		return "amd"
	case "8086":
		return "intel"
	default:
		return ""
	}
}

// DeviceLabel normalizes a PCI device name into a label-safe value.
func DeviceLabel(deviceName string) string {
	if deviceName == "" {
		return ""
	}

	name := deviceName
	if start := strings.Index(deviceName, "["); start >= 0 {
		if end := strings.Index(deviceName[start+1:], "]"); end >= 0 {
			name = deviceName[start+1 : start+1+end]
		}
	}

	return normalizeLabelValue(name)
}

func normalizeLabelValue(value string) string {
	if value == "" {
		return ""
	}

	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	return out
}
